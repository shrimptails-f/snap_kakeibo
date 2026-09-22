package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"time"

	analysisapp "snap_kakeibo/backend/internal/analysis/application"
	analysisdomain "snap_kakeibo/backend/internal/analysis/domain"
	analysisinfra "snap_kakeibo/backend/internal/analysis/infrastructure"
	analysisimage "snap_kakeibo/backend/internal/analysis/library/image"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/logger"
	libs3 "snap_kakeibo/backend/internal/library/s3"
	"snap_kakeibo/backend/internal/library/timewrapper"
	uploadapp "snap_kakeibo/backend/internal/upload/application"
	uploadinfra "snap_kakeibo/backend/internal/upload/infrastructure"
	"snap_kakeibo/backend/tools/localenv"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// sampleOutcome はサンプルの解析依頼をどの状態で止めるか。
type sampleOutcome string

const (
	outcomeSucceeded sampleOutcome = "SUCCEEDED"
	outcomeNoData    sampleOutcome = "NO_DATA"
	outcomeFailed    sampleOutcome = "FAILED"
	outcomeAnalyzing sampleOutcome = "ANALYZING"
	outcomeUploading sampleOutcome = "UPLOADING"
)

// sample は 1 件の解析依頼。monthOffset は今月からの月差(0 が今月、-1 が先月)、day はその月の日。
// SUCCEEDED のときだけ store / amount / details を支出として登録する。
type sample struct {
	monthOffset int
	day         int
	outcome     sampleOutcome
	fileName    string
	store       string
	amount      int64
	details     []analysisdomain.ReadDetail
}

// samples は画面確認用の固定データ。今月は全状態を含み、先月・先々月は登録済みの支出だけを入れる。
// 順番と内容が ID になる(sampleID)ので、入れ替えると別の依頼として追加される。
var samples = []sample{
	{monthOffset: 0, day: 2, outcome: outcomeSucceeded, fileName: "IMG_0001.jpg", store: "まいばすけっと 中野店", amount: 1284, details: []analysisdomain.ReadDetail{
		{Name: "牛乳 1000ml", Amount: 238, Quantity: 1, Category: "food"},
		{Name: "食パン 6枚切", Amount: 158, Quantity: 1, Category: "food"},
		{Name: "たまご 10個", Amount: 298, Quantity: 1, Category: "food"},
		{Name: "ヨーグルト", Amount: 148, Quantity: 2, Category: "food"},
		{Name: "キッチンペーパー", Amount: 294, Quantity: 1, Category: "daily_goods"},
	}},
	{monthOffset: 0, day: 5, outcome: outcomeSucceeded, fileName: "IMG_0002.jpg", store: "マツモトキヨシ", amount: 2156, details: []analysisdomain.ReadDetail{
		{Name: "ロキソニンS 12錠", Amount: 767, Quantity: 1, Category: "medical"},
		{Name: "シャンプー詰替", Amount: 698, Quantity: 1, Category: "daily_goods"},
		{Name: "歯ブラシ", Amount: 328, Quantity: 1, Category: "daily_goods"},
		{Name: "のど飴", Amount: 363, Quantity: 1, Category: "food"},
	}},
	{monthOffset: 0, day: 9, outcome: outcomeSucceeded, fileName: "IMG_0003.jpg", store: "JR東日本", amount: 1520, details: []analysisdomain.ReadDetail{
		{Name: "Suica チャージ", Amount: 1520, Quantity: 1, Category: "transport"},
	}},
	{monthOffset: 0, day: 12, outcome: outcomeSucceeded, fileName: "IMG_0004.jpg", store: "サイゼリヤ", amount: 2200, details: []analysisdomain.ReadDetail{
		{Name: "ミラノ風ドリア", Amount: 300, Quantity: 2, Category: "social"},
		{Name: "辛味チキン", Amount: 300, Quantity: 1, Category: "social"},
		{Name: "グラスワイン", Amount: 200, Quantity: 2, Category: "social"},
		{Name: "ドリンクバー", Amount: 300, Quantity: 2, Category: "social"},
		{Name: "ティラミス", Amount: 300, Quantity: 1, Category: "social"},
	}},
	{monthOffset: 0, day: 15, outcome: outcomeSucceeded, fileName: "IMG_0005.jpg", store: "ユニクロ", amount: 4980, details: []analysisdomain.ReadDetail{
		{Name: "ヒートテッククルーネックT", Amount: 1500, Quantity: 2, Category: "clothing"},
		{Name: "スウェットパンツ", Amount: 1980, Quantity: 1, Category: "clothing"},
	}},
	{monthOffset: 0, day: 16, outcome: outcomeNoData, fileName: "IMG_0006.jpg", store: "駐車場 領収書", amount: 800},
	{monthOffset: 0, day: 17, outcome: outcomeFailed, fileName: "IMG_0007.jpg"},
	{monthOffset: 0, day: 18, outcome: outcomeAnalyzing, fileName: "IMG_0008.jpg"},
	{monthOffset: 0, day: 19, outcome: outcomeUploading, fileName: "IMG_0009.jpg"},

	{monthOffset: -1, day: 3, outcome: outcomeSucceeded, fileName: "IMG_0101.jpg", store: "西友", amount: 3462, details: []analysisdomain.ReadDetail{
		{Name: "鶏むね肉 2kg", Amount: 1180, Quantity: 1, Category: "food"},
		{Name: "米 5kg", Amount: 1980, Quantity: 1, Category: "food"},
		{Name: "ラップ", Amount: 302, Quantity: 1, Category: "daily_goods"},
	}},
	{monthOffset: -1, day: 10, outcome: outcomeSucceeded, fileName: "IMG_0102.jpg", store: "東京電力", amount: 6830, details: []analysisdomain.ReadDetail{
		{Name: "電気料金", Amount: 6830, Quantity: 1, Category: "utilities"},
	}},
	{monthOffset: -1, day: 14, outcome: outcomeSucceeded, fileName: "IMG_0103.jpg", store: "TOHOシネマズ", amount: 2000, details: []analysisdomain.ReadDetail{
		{Name: "映画チケット", Amount: 2000, Quantity: 1, Category: "entertainment"},
	}},
	{monthOffset: -1, day: 22, outcome: outcomeSucceeded, fileName: "IMG_0104.jpg", store: "紀伊國屋書店", amount: 3520, details: []analysisdomain.ReadDetail{
		{Name: "技術書", Amount: 3520, Quantity: 1, Category: "education"},
	}},
	{monthOffset: -1, day: 27, outcome: outcomeSucceeded, fileName: "IMG_0105.jpg", store: "セブン-イレブン", amount: 764, details: []analysisdomain.ReadDetail{
		{Name: "おにぎり", Amount: 150, Quantity: 2, Category: "food"},
		{Name: "コーヒー", Amount: 120, Quantity: 1, Category: "food"},
		{Name: "コピー", Amount: 20, Quantity: 3, Category: "other"},
		{Name: "乾電池", Amount: 284, Quantity: 1, Category: "daily_goods"},
	}},

	{monthOffset: -2, day: 4, outcome: outcomeSucceeded, fileName: "IMG_0201.jpg", store: "イオン", amount: 5890, details: []analysisdomain.ReadDetail{
		{Name: "食料品 まとめ買い", Amount: 4320, Quantity: 1, Category: "food"},
		{Name: "洗剤", Amount: 570, Quantity: 1, Category: "daily_goods"},
		{Name: "靴下 3足", Amount: 1000, Quantity: 1, Category: "clothing"},
	}},
	{monthOffset: -2, day: 11, outcome: outcomeSucceeded, fileName: "IMG_0202.jpg", store: "東京ガス", amount: 4120, details: []analysisdomain.ReadDetail{
		{Name: "ガス料金", Amount: 4120, Quantity: 1, Category: "utilities"},
	}},
	{monthOffset: -2, day: 20, outcome: outcomeSucceeded, fileName: "IMG_0203.jpg", store: "ダイソー", amount: 660, details: []analysisdomain.ReadDetail{
		{Name: "収納ボックス", Amount: 110, Quantity: 3, Category: "daily_goods"},
		{Name: "メモ帳", Amount: 110, Quantity: 3, Category: "other"},
	}},
}

// seedSamples はサンプルを、本番と同じ upload / analysis の usecase(OpenAI だけ固定応答)で登録する。
// 既に同じ ID の解析依頼があればその件は飛ばす。
func seedSamples(ctx context.Context, cfg aws.Config, userID string) error {
	log := logger.NewNop()
	ddb := libdynamodb.New(cfg, log)
	requests := ddb.Table(localenv.TableName(libdynamodb.AnalysisRequestsSchema))
	bucket := libs3.New(cfg, log).Bucket(localenv.ReceiptBucket)
	storage := analysisinfra.S3ReceiptStorage{Client: libs3.New(cfg, log), Results: bucket}
	registrar := analysisinfra.DynamoDBExpenseRegistrar{
		Client:           ddb,
		AnalysisRequests: requests,
		Expenses:         ddb.Table(localenv.TableName(libdynamodb.ExpensesSchema)),
		ExpenseDetails:   ddb.Table(localenv.TableName(libdynamodb.ExpenseDetailsSchema)),
		MonthlySummaries: ddb.Table(localenv.TableName(libdynamodb.MonthlySummariesSchema)),
	}

	now := time.Now()
	created, skipped := 0, 0
	for i, s := range samples {
		id := sampleID(now, s, i)
		exists, err := analysisRequestExists(ctx, requests, userID, id)
		if err != nil {
			return err
		}
		if exists {
			skipped++
			continue
		}
		at := sampleTime(now, s)
		clock := timewrapper.NewFixed(at)

		// 1. 解析依頼の登録(UPLOADING)。署名付き URL は使わず、画像は直接置く
		upload := uploadapp.NewCreateUploadUsecase(
			uploadinfra.DynamoDBAnalysisRequestRepository{Table: requests},
			uploadinfra.S3UploadPresigner{Bucket: bucket},
			fixedIDs{id},
			clock,
		)
		uploaded, err := upload.Create(ctx, uploadapp.CreateUploadInput{UserID: userID, FileName: s.fileName, ContentType: "image/jpeg"})
		if err != nil {
			return fmt.Errorf("sample %s: create upload: %w", id, err)
		}
		if s.outcome == outcomeUploading {
			created++
			continue
		}
		if err := bucket.PutBytes(ctx, uploaded.S3Key, samplePNG(), "image/png"); err != nil {
			return fmt.Errorf("sample %s: put image: %w", id, err)
		}

		// 2. 解析。ANALYZING は MarkAnalyzing で止め、それ以外は固定応答の analyzer で終端まで進める
		job := analysisdomain.AnalysisJob{
			UserID: userID, AnalysisRequestID: id, Attempt: 1,
			Trigger: analysisdomain.TriggerS3, Bucket: localenv.ReceiptBucket, Key: uploaded.S3Key,
		}
		if s.outcome == outcomeAnalyzing {
			if _, err := (analysisinfra.DynamoDBAnalysisRequestRepository{Table: requests}).MarkAnalyzing(ctx, job, at); err != nil {
				return fmt.Errorf("sample %s: mark analyzing: %w", id, err)
			}
			created++
			continue
		}
		analyze := analysisapp.NewAnalyzeReceiptUsecase(
			storage,
			analysisimage.Resizer{},
			sampleAnalyzer{sample: s, purchaseDate: at.Format("2006-01-02"), responseID: "resp-" + id},
			storage,
			analysisinfra.DynamoDBAnalysisRequestRepository{Table: requests},
			registrar,
			sampleDetailIDs(id, len(s.details)),
			clock,
			log,
		)
		out, err := analyze.Analyze(ctx, analysisapp.AnalyzeReceiptInput{Job: job})
		if err != nil {
			return fmt.Errorf("sample %s: analyze: %w", id, err)
		}
		if string(out.Outcome) != string(s.outcome) {
			return fmt.Errorf("sample %s: outcome = %s, want %s", id, out.Outcome, s.outcome)
		}
		created++
	}
	fmt.Printf("samples:  %d created, %d already present\n", created, skipped)
	return nil
}

// sampleID は「対象月 + 通し番号」で決まる固定 ID。月が変わると今月分は新しい ID になり、前の月の分はそのまま残る。
func sampleID(now time.Time, s sample, index int) string {
	return fmt.Sprintf("sample-%s-%02d", sampleTime(now, s).Format("200601"), index+1)
}

// sampleTime は対象月の day 日 12:00。今月で day が今日より先なら今日にする(購入日は翌日までしか登録できない)。
func sampleTime(now time.Time, s sample) time.Time {
	first := time.Date(now.Year(), now.Month()+time.Month(s.monthOffset), 1, 12, 0, 0, 0, now.Location())
	day := s.day
	if s.monthOffset == 0 && day > now.Day() {
		day = now.Day()
	}
	return first.AddDate(0, 0, day-1)
}

func analysisRequestExists(ctx context.Context, requests *libdynamodb.Table, userID, id string) (bool, error) {
	out, err := requests.GetItem(ctx, &awsdynamodb.GetItemInput{Key: map[string]ddbtypes.AttributeValue{
		"PK": &ddbtypes.AttributeValueMemberS{Value: analysisinfra.UserPK(userID)},
		"SK": &ddbtypes.AttributeValueMemberS{Value: analysisinfra.AnalysisRequestSK(id)},
	}})
	if err != nil {
		return false, fmt.Errorf("get analysis request %s: %w", id, err)
	}
	return len(out.Item) > 0, nil
}

// fixedIDs は 1 つの ID だけを返す採番器(解析依頼 ID 用)。
type fixedIDs struct{ id string }

func (f fixedIDs) NewID() (string, error) { return f.id, nil }

// sampleDetailIDs は analyze usecase が使う順(expense_id、次に detail_id × 件数)で ID を返す。
func sampleDetailIDs(requestID string, details int) *sequenceIDs {
	ids := []string{"exp-" + requestID}
	for i := range details {
		ids = append(ids, fmt.Sprintf("det-%s-%d", requestID, i+1))
	}
	return &sequenceIDs{values: ids}
}

type sequenceIDs struct {
	values []string
	next   int
}

func (s *sequenceIDs) NewID() (string, error) {
	if s.next >= len(s.values) {
		return "", fmt.Errorf("sample ID sequence exhausted")
	}
	id := s.values[s.next]
	s.next++
	return id, nil
}

// sampleAnalyzer は OpenAI の代わりに sample の内容をそのまま読み取り結果として返す。
type sampleAnalyzer struct {
	sample       sample
	purchaseDate string
	responseID   string
}

func (a sampleAnalyzer) Analyze(_ context.Context, _ []byte) (analysisapp.AnalyzerResponse, error) {
	res := analysisapp.AnalyzerResponse{
		ResponseID: a.responseID,
		Raw:        []byte(fmt.Sprintf(`{"id":%q,"sample":true}`, a.responseID)),
	}
	if a.sample.outcome == outcomeFailed {
		failure := analysisdomain.AnalysisFailed("レシートとして読み取れませんでした")
		res.Failure = &failure
		return res, nil
	}
	store, date, amount := a.sample.store, a.purchaseDate, a.sample.amount
	res.Reading = analysisdomain.ReceiptReading{StoreName: &store, PurchaseDate: &date, ReadAmount: &amount, Details: a.sample.details}
	return res, nil
}

// samplePNG は解析の入力として置く小さな画像。中身は見ないので単色でよい。
func samplePNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.Set(x, y, color.RGBA{R: 240, G: 240, B: 232, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
