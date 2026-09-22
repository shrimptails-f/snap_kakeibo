// localapi は API Gateway の代わりに HTTP を受け、cmd/ の各 Lambda をローカルで呼び出す開発用サーバー。
//
// Lambda のハンドラは package main にあり import できないため、cmd/<name> をビルドして
// aws-lambda-go のローカル RPC モード(_LAMBDA_SERVER_PORT)で子プロセスとして起動し、net/rpc で呼ぶ。
// init() や lambdawrap を含めて本番と同じコードパスが動く。
//
//	go run ./tools/localseed   # 先に Floci へリソースと利用者を作る
//	go run ./tools/localapi    # :8080 で待ち受ける。front は VITE_DEV_API_PROXY=http://localhost:8080
//
// 受け付けるルートは infra/stacks/app.go の addRoute と同じ。analyze-receipt(SQS 起動)は動かさないので、
// アップロードした画像は UPLOADING のまま残る。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"snap_kakeibo/backend/internal/library/awsconfig"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/tools/localenv"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
)

// routes は infra/stacks/app.go の addRoute と同じ対応。パスパラメータ名も API Gateway と揃える。
var routes = []route{
	{method: http.MethodGet, path: "/api/hello", function: "hello"},
	{method: http.MethodPost, path: "/api/auth/login", function: "auth-login"},
	{method: http.MethodPost, path: "/api/auth/refresh", function: "auth-refresh"},
	{method: http.MethodPost, path: "/api/auth/logout", function: "auth-logout"},
	{method: http.MethodGet, path: "/api/auth/check", function: "auth-check"},
	{method: http.MethodPost, path: "/api/uploads", function: "upload", transform: rewritePutURL},
	{method: http.MethodPost, path: "/api/analysis-requests/{analysisRequestId}/retry", function: "retry-analysis"},
	{method: http.MethodGet, path: "/api/months/{month}/analysis-requests", function: "list-analysis-requests"},
	{method: http.MethodGet, path: "/api/monthly-summaries", function: "get-monthly-summaries"},
	{method: http.MethodGet, path: "/api/months/{month}/expenses", function: "list-month-expenses"},
	{method: http.MethodGet, path: "/api/expenses/{expenseId}", function: "get-expense", transform: rewriteExpenseImageURL},
}

// s3ProxyPrefix はブラウザからの presigned PUT を Floci へ中継するパス。
const s3ProxyPrefix = "/s3"

func main() {
	addr := flag.String("addr", ":8080", "待ち受けアドレス")
	publicURL := flag.String("public-url", "http://localhost:8080", "ブラウザから見たこのサーバーの origin。presigned URL の向き先に使う")
	invokeTimeout := flag.Duration("timeout", 30*time.Second, "Lambda 1 回の呼び出し上限")
	flag.Parse()

	if err := run(*addr, *publicURL, *invokeTimeout); err != nil {
		log.Fatalf("localapi: %v", err)
	}
}

func run(addr, publicURL string, invokeTimeout time.Duration) error {
	if err := localenv.EnsureLocalProcessEnv(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := awsconfig.Load(ctx, oswrapper.New())
	if err != nil {
		return err
	}
	endpoint := aws.ToString(cfg.BaseEndpoint)
	queueURL, err := analyzeQueueURL(ctx, cfg)
	if err != nil {
		return err
	}
	environment := localenv.LambdaEnvironment(endpoint, cfg.Region, queueURL)
	if level := os.Getenv(localenv.EnvLogLevel); level != "" {
		environment[localenv.EnvLogLevel] = level
	}

	pool, err := startFunctions(ctx, functionNames(), environment, invokeTimeout)
	if err != nil {
		return err
	}
	defer pool.Close()

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	for _, rt := range routes {
		mux.Handle(rt.method+" "+rt.path, newGatewayHandler(pool, rt, transformContext{endpoint: endpoint, publicS3URL: strings.TrimSuffix(publicURL, "/") + s3ProxyPrefix}))
	}
	mux.Handle(s3ProxyPrefix+"/", newS3Proxy(s3ProxyPrefix, endpointURL))
	mux.HandleFunc("/", notFound)

	server := &http.Server{Addr: addr, Handler: logRequests(mux), ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	log.Printf("listening on %s (floci: %s, public: %s)", addr, endpoint, publicURL)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Print("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

// analyzeQueueURL は seed が作ったキューの URL を引く。無ければ seed 未実行なので案内して止まる。
func analyzeQueueURL(ctx context.Context, cfg aws.Config) (string, error) {
	out, err := awssqs.NewFromConfig(cfg).GetQueueUrl(ctx, &awssqs.GetQueueUrlInput{QueueName: aws.String(localenv.AnalyzeQueueName)})
	if err != nil {
		return "", fmt.Errorf("queue %s not found (run `go run ./tools/localseed` first): %w", localenv.AnalyzeQueueName, err)
	}
	return aws.ToString(out.QueueUrl), nil
}

func functionNames() []string {
	seen := map[string]bool{}
	var names []string
	for _, rt := range routes {
		if !seen[rt.function] {
			seen[rt.function] = true
			names = append(names, rt.function)
		}
	}
	return names
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, `{"message":"Not Found"}`)
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// logRequests は 1 リクエスト 1 行のアクセスログを出す。
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.RequestURI(), recorder.status, time.Since(started).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}
