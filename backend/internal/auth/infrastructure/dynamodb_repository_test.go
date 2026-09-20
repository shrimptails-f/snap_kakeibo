package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/auth/application"
	authdomain "snap_kakeibo/backend/internal/auth/domain"
	common "snap_kakeibo/backend/internal/common/domain"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestDynamoDBUserRepositoryFindByEmail(t *testing.T) {
	item, err := attributevalue.MarshalMap(userRecord{
		UserID:       "user-123",
		Email:        "member@example.com",
		PasswordHash: "hashed-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	api := &repositoryAPI{getOutput: &awssdk.GetItemOutput{Item: item}}
	repository := DynamoDBUserRepository{Table: libdynamodb.NewWithAPI(api).Table("users-test")}

	user, err := repository.FindByEmail(context.Background(), " MEMBER@Example.COM ")
	if err != nil {
		t.Fatalf("FindByEmail() error = %v", err)
	}
	if user != (common.User{ID: "user-123", Email: "member@example.com", PasswordHash: "hashed-password"}) {
		t.Errorf("FindByEmail() user = %#v", user)
	}
	if got := aws.ToString(api.getInput.TableName); got != "users-test" {
		t.Errorf("table name = %q, want users-test", got)
	}
	if got := api.getInput.Key["PK"]; got == nil {
		t.Fatal("GetItem key does not contain PK")
	} else if value := got.(*ddbtypes.AttributeValueMemberS).Value; value != "EMAIL#member@example.com" {
		t.Errorf("PK = %q, want %q", value, "EMAIL#member@example.com")
	}
}

func TestDynamoDBUserRepositoryFindByEmailFailures(t *testing.T) {
	validItem, err := attributevalue.MarshalMap(userRecord{UserID: "user-1", Email: "member@example.com", PasswordHash: "hash"})
	if err != nil {
		t.Fatal(err)
	}
	malformedItem := map[string]ddbtypes.AttributeValue{
		"user_id":       &ddbtypes.AttributeValueMemberSS{Value: []string{"user-1"}},
		"email":         &ddbtypes.AttributeValueMemberS{Value: "member@example.com"},
		"password_hash": &ddbtypes.AttributeValueMemberS{Value: "hash"},
	}
	sdkErr := errors.New("DynamoDB unavailable")

	for _, tt := range []struct {
		name    string
		output  *awssdk.GetItemOutput
		err     error
		wantErr error
	}{
		{name: "not found", output: &awssdk.GetItemOutput{}, wantErr: application.ErrUserNotFound},
		{name: "blank user ID", output: &awssdk.GetItemOutput{Item: map[string]ddbtypes.AttributeValue{
			"user_id": &ddbtypes.AttributeValueMemberS{Value: "  "}, "password_hash": &ddbtypes.AttributeValueMemberS{Value: "hash"},
		}}, wantErr: application.ErrUserNotFound},
		{name: "blank password hash", output: &awssdk.GetItemOutput{Item: map[string]ddbtypes.AttributeValue{
			"user_id": &ddbtypes.AttributeValueMemberS{Value: "user-1"}, "password_hash": &ddbtypes.AttributeValueMemberS{Value: " \t"},
		}}, wantErr: application.ErrUserNotFound},
		{name: "malformed item", output: &awssdk.GetItemOutput{Item: malformedItem}},
		{name: "DynamoDB error", output: &awssdk.GetItemOutput{Item: validItem}, err: sdkErr, wantErr: sdkErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := &repositoryAPI{getOutput: tt.output, getErr: tt.err}
			repository := DynamoDBUserRepository{Table: libdynamodb.NewWithAPI(api).Table("users-test")}

			_, err := repository.FindByEmail(context.Background(), "member@example.com")
			if err == nil {
				t.Fatal("FindByEmail() error = nil")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("FindByEmail() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDynamoDBRefreshTokenRepositorySave(t *testing.T) {
	api := &repositoryAPI{putOutput: &awssdk.PutItemOutput{}}
	repository := DynamoDBRefreshTokenRepository{Table: libdynamodb.NewWithAPI(api).Table("users-test")}
	createdAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	token := authdomain.RefreshToken{
		Digest: "digest", UserID: "user-123", Email: "member@example.com",
		ExpiresAt: createdAt.Add(30 * 24 * time.Hour), CreatedAt: createdAt,
		LastUsedAt: createdAt.Add(time.Hour), Hint: "abcd",
	}

	if err := repository.Save(context.Background(), token); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if got := aws.ToString(api.putInput.TableName); got != "users-test" {
		t.Errorf("table name = %q, want users-test", got)
	}
	if got := aws.ToString(api.putInput.ConditionExpression); got != "attribute_not_exists(PK)" {
		t.Errorf("condition expression = %q", got)
	}
	var record refreshTokenRecord
	if err := attributevalue.UnmarshalMap(api.putInput.Item, &record); err != nil {
		t.Fatalf("UnmarshalMap() error = %v", err)
	}
	if record.PK != "REFRESH#digest" || record.Type != "REFRESH_TOKEN" || record.UserID != "user-123" || record.Email != token.Email || record.RefreshHint != token.Hint {
		t.Errorf("saved record = %#v", record)
	}
	if record.GSI1PK != "USER#user-123" || record.GSI1SK != "REFRESH_CREATED_AT#2026-09-20T03:00:00Z#digest" {
		t.Errorf("saved GSI keys = %q / %q", record.GSI1PK, record.GSI1SK)
	}
	// expires_at は TTL 属性なので Unix 秒の数値で保存する
	if n, ok := api.putInput.Item["expires_at"].(*ddbtypes.AttributeValueMemberN); !ok || n.Value != fmt.Sprint(token.ExpiresAt.Unix()) {
		t.Errorf("expires_at = %#v, want N %d", api.putInput.Item["expires_at"], token.ExpiresAt.Unix())
	}
	if !record.CreatedAt.Equal(token.CreatedAt.UTC()) || !record.LastUsedAt.Equal(token.LastUsedAt.UTC()) {
		t.Errorf("saved timestamps = %#v", record)
	}
	if record.RevokedAt != nil {
		t.Errorf("revoked_at is written on save: %v", record.RevokedAt)
	}
}

func TestDynamoDBRefreshTokenRepositorySaveReturnsDynamoDBError(t *testing.T) {
	sdkErr := errors.New("conditional check failed")
	api := &repositoryAPI{putOutput: &awssdk.PutItemOutput{}, putErr: sdkErr}
	repository := DynamoDBRefreshTokenRepository{Table: libdynamodb.NewWithAPI(api).Table("users-test")}

	err := repository.Save(context.Background(), authdomain.RefreshToken{Digest: "digest"})
	if !errors.Is(err, sdkErr) {
		t.Errorf("Save() error = %v, want %v", err, sdkErr)
	}
}

func TestDynamoDBRefreshTokenRepositoryFindByDigest(t *testing.T) {
	createdAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	item, err := attributevalue.MarshalMap(refreshTokenRecord{
		PK: "REFRESH#digest", Type: "REFRESH_TOKEN", UserID: "user-123", Email: "member@example.com",
		ExpiresAt: createdAt.Add(30 * 24 * time.Hour).Unix(), CreatedAt: createdAt, LastUsedAt: createdAt, RefreshHint: "abcd",
	})
	if err != nil {
		t.Fatal(err)
	}
	api := &repositoryAPI{getOutput: &awssdk.GetItemOutput{Item: item}}
	repository := DynamoDBRefreshTokenRepository{Table: libdynamodb.NewWithAPI(api).Table("users-test")}

	token, err := repository.FindByDigest(context.Background(), "digest")
	if err != nil {
		t.Fatalf("FindByDigest() error = %v", err)
	}
	if got := aws.ToString(api.getInput.TableName); got != "users-test" {
		t.Errorf("table name = %q, want users-test", got)
	}
	if got := api.getInput.Key["PK"].(*ddbtypes.AttributeValueMemberS).Value; got != "REFRESH#digest" {
		t.Errorf("PK = %q, want REFRESH#digest", got)
	}
	if !aws.ToBool(api.getInput.ConsistentRead) {
		t.Error("GetItem is not a consistent read")
	}
	if token.Digest != "digest" || token.UserID != "user-123" || token.Email != "member@example.com" || token.Hint != "abcd" {
		t.Errorf("token = %#v", token)
	}
	if !token.ExpiresAt.Equal(createdAt.Add(30*24*time.Hour)) || !token.CreatedAt.Equal(createdAt) || !token.LastUsedAt.Equal(createdAt) {
		t.Errorf("token timestamps = %#v", token)
	}
	if token.Revoked() {
		t.Errorf("token without revoked_at is revoked: %#v", token)
	}
}

func TestDynamoDBRefreshTokenRepositoryFindByDigestReadsRevokedAt(t *testing.T) {
	revokedAt := time.Date(2026, 9, 20, 13, 0, 0, 0, time.UTC)
	item, err := attributevalue.MarshalMap(refreshTokenRecord{PK: "REFRESH#digest", UserID: "user-123", RevokedAt: &revokedAt})
	if err != nil {
		t.Fatal(err)
	}
	api := &repositoryAPI{getOutput: &awssdk.GetItemOutput{Item: item}}
	repository := DynamoDBRefreshTokenRepository{Table: libdynamodb.NewWithAPI(api).Table("users-test")}

	token, err := repository.FindByDigest(context.Background(), "digest")
	if err != nil {
		t.Fatalf("FindByDigest() error = %v", err)
	}
	if !token.Revoked() || !token.RevokedAt.Equal(revokedAt) {
		t.Errorf("revoked at = %v, want %v", token.RevokedAt, revokedAt)
	}
}

func TestDynamoDBRefreshTokenRepositoryFindByDigestFailures(t *testing.T) {
	sdkErr := errors.New("DynamoDB unavailable")
	for _, tt := range []struct {
		name    string
		output  *awssdk.GetItemOutput
		err     error
		wantErr error
	}{
		{name: "not found", output: &awssdk.GetItemOutput{}, wantErr: application.ErrRefreshTokenNotFound},
		{name: "blank user ID", output: &awssdk.GetItemOutput{Item: map[string]ddbtypes.AttributeValue{
			"user_id": &ddbtypes.AttributeValueMemberS{Value: " "},
		}}, wantErr: application.ErrRefreshTokenNotFound},
		{name: "malformed item", output: &awssdk.GetItemOutput{Item: map[string]ddbtypes.AttributeValue{
			"user_id": &ddbtypes.AttributeValueMemberSS{Value: []string{"user-1"}},
		}}},
		{name: "DynamoDB error", output: &awssdk.GetItemOutput{}, err: sdkErr, wantErr: sdkErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := &repositoryAPI{getOutput: tt.output, getErr: tt.err}
			repository := DynamoDBRefreshTokenRepository{Table: libdynamodb.NewWithAPI(api).Table("users-test")}

			_, err := repository.FindByDigest(context.Background(), "digest")
			if err == nil {
				t.Fatal("FindByDigest() error = nil")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("FindByDigest() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDynamoDBRefreshTokenRepositoryRevoke(t *testing.T) {
	api := &repositoryAPI{updateOutput: &awssdk.UpdateItemOutput{}}
	repository := DynamoDBRefreshTokenRepository{Table: libdynamodb.NewWithAPI(api).Table("users-test")}
	revokedAt := time.Date(2026, 9, 20, 21, 0, 0, 0, time.FixedZone("JST", 9*60*60))

	if err := repository.Revoke(context.Background(), "digest", revokedAt); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if got := aws.ToString(api.updateInput.TableName); got != "users-test" {
		t.Errorf("table name = %q, want users-test", got)
	}
	if got := api.updateInput.Key["PK"].(*ddbtypes.AttributeValueMemberS).Value; got != "REFRESH#digest" {
		t.Errorf("PK = %q, want REFRESH#digest", got)
	}
	if got := aws.ToString(api.updateInput.ConditionExpression); got != "attribute_exists(PK)" {
		t.Errorf("condition expression = %q", got)
	}
	var stored time.Time
	if err := attributevalue.Unmarshal(api.updateInput.ExpressionAttributeValues[":revoked_at"], &stored); err != nil {
		t.Fatalf("Unmarshal(:revoked_at) error = %v", err)
	}
	if !stored.Equal(revokedAt) {
		t.Errorf(":revoked_at = %v, want %v", stored, revokedAt)
	}
}

func TestDynamoDBRefreshTokenRepositoryRevokeFailures(t *testing.T) {
	sdkErr := errors.New("DynamoDB unavailable")
	for _, tt := range []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "missing item", err: &ddbtypes.ConditionalCheckFailedException{}, wantErr: application.ErrRefreshTokenNotFound},
		{name: "DynamoDB error", err: sdkErr, wantErr: sdkErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := &repositoryAPI{updateErr: tt.err}
			repository := DynamoDBRefreshTokenRepository{Table: libdynamodb.NewWithAPI(api).Table("users-test")}

			err := repository.Revoke(context.Background(), "digest", time.Now())
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Revoke() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDynamoDBRefreshTokenRepositoryRevokeAllByUser(t *testing.T) {
	pk := func(v string) map[string]ddbtypes.AttributeValue {
		return map[string]ddbtypes.AttributeValue{"PK": &ddbtypes.AttributeValueMemberS{Value: v}}
	}
	// 2 ページに分かれた Query 結果を順に返す
	api := &repositoryAPI{
		queryOutputs: []*awssdk.QueryOutput{
			{Items: []map[string]ddbtypes.AttributeValue{pk("REFRESH#a"), pk("REFRESH#b")}, LastEvaluatedKey: pk("REFRESH#b")},
			{Items: []map[string]ddbtypes.AttributeValue{pk("REFRESH#c")}},
		},
		updateOutput: &awssdk.UpdateItemOutput{},
	}
	repository := DynamoDBRefreshTokenRepository{Table: libdynamodb.NewWithAPI(api).Table("refresh-tokens-test")}
	revokedAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	if err := repository.RevokeAllByUser(context.Background(), "user-123", revokedAt); err != nil {
		t.Fatalf("RevokeAllByUser() error = %v", err)
	}
	if len(api.queryInputs) != 2 {
		t.Fatalf("Query called %d times, want 2", len(api.queryInputs))
	}
	first := api.queryInputs[0]
	if aws.ToString(first.TableName) != "refresh-tokens-test" || aws.ToString(first.IndexName) != libdynamodb.RefreshTokenUserIndex {
		t.Errorf("query target = %s / %s", aws.ToString(first.TableName), aws.ToString(first.IndexName))
	}
	if got := first.ExpressionAttributeValues[":user"].(*ddbtypes.AttributeValueMemberS).Value; got != "USER#user-123" {
		t.Errorf(":user = %q", got)
	}
	if aws.ToString(first.FilterExpression) != "attribute_not_exists(revoked_at)" {
		t.Errorf("filter expression = %q", aws.ToString(first.FilterExpression))
	}
	if first.ExclusiveStartKey != nil {
		t.Errorf("first page has ExclusiveStartKey %v", first.ExclusiveStartKey)
	}
	if got := api.queryInputs[1].ExclusiveStartKey["PK"].(*ddbtypes.AttributeValueMemberS).Value; got != "REFRESH#b" {
		t.Errorf("second page ExclusiveStartKey = %q", got)
	}
	var revoked []string
	for _, in := range api.updateInputs {
		revoked = append(revoked, in.Key["PK"].(*ddbtypes.AttributeValueMemberS).Value)
		if aws.ToString(in.ConditionExpression) != "attribute_exists(PK)" {
			t.Errorf("condition expression = %q", aws.ToString(in.ConditionExpression))
		}
	}
	if strings.Join(revoked, ",") != "REFRESH#a,REFRESH#b,REFRESH#c" {
		t.Errorf("revoked = %v", revoked)
	}
}

func TestDynamoDBRefreshTokenRepositoryRevokeAllByUserSkipsExpiredItems(t *testing.T) {
	api := &repositoryAPI{
		queryOutputs: []*awssdk.QueryOutput{{Items: []map[string]ddbtypes.AttributeValue{
			{"PK": &ddbtypes.AttributeValueMemberS{Value: "REFRESH#gone"}},
		}}},
		// 一覧取得後に TTL で消えたアイテムは条件不一致になる
		updateErr: &ddbtypes.ConditionalCheckFailedException{},
	}
	repository := DynamoDBRefreshTokenRepository{Table: libdynamodb.NewWithAPI(api).Table("refresh-tokens-test")}

	if err := repository.RevokeAllByUser(context.Background(), "user-123", time.Now()); err != nil {
		t.Fatalf("RevokeAllByUser() error = %v", err)
	}
}

func TestDynamoDBRefreshTokenRepositoryRevokeAllByUserFailures(t *testing.T) {
	sdkErr := errors.New("DynamoDB unavailable")
	for _, tt := range []struct {
		name string
		api  *repositoryAPI
	}{
		{name: "query error", api: &repositoryAPI{queryErr: sdkErr}},
		{name: "update error", api: &repositoryAPI{
			queryOutputs: []*awssdk.QueryOutput{{Items: []map[string]ddbtypes.AttributeValue{
				{"PK": &ddbtypes.AttributeValueMemberS{Value: "REFRESH#a"}},
			}}},
			updateErr: sdkErr,
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repository := DynamoDBRefreshTokenRepository{Table: libdynamodb.NewWithAPI(tt.api).Table("refresh-tokens-test")}
			if err := repository.RevokeAllByUser(context.Background(), "user-123", time.Now()); !errors.Is(err, sdkErr) {
				t.Errorf("RevokeAllByUser() error = %v, want %v", err, sdkErr)
			}
		})
	}
}

func TestPersistenceKeys(t *testing.T) {
	if got := UserPKByEmail(" MEMBER@Example.COM "); got != "EMAIL#member@example.com" {
		t.Errorf("UserPKByEmail() = %q", got)
	}
	if got := RefreshPK("digest"); got != "REFRESH#digest" {
		t.Errorf("RefreshPK() = %q", got)
	}
	if got := RefreshUserGSI1PK("user-1"); got != "USER#user-1" {
		t.Errorf("RefreshUserGSI1PK() = %q", got)
	}
	createdAt := time.Date(2026, 9, 20, 21, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	if got := RefreshCreatedGSI1SK(createdAt, "digest"); got != "REFRESH_CREATED_AT#2026-09-20T12:00:00Z#digest" {
		t.Errorf("RefreshCreatedGSI1SK() = %q", got)
	}
}

type repositoryAPI struct {
	getInput  *awssdk.GetItemInput
	getOutput *awssdk.GetItemOutput
	getErr    error
	putInput  *awssdk.PutItemInput
	putOutput *awssdk.PutItemOutput
	putErr    error

	updateInput  *awssdk.UpdateItemInput
	updateInputs []*awssdk.UpdateItemInput
	updateOutput *awssdk.UpdateItemOutput
	updateErr    error

	queryInputs  []*awssdk.QueryInput
	queryOutputs []*awssdk.QueryOutput
	queryErr     error
}

func (a *repositoryAPI) GetItem(_ context.Context, in *awssdk.GetItemInput, _ ...func(*awssdk.Options)) (*awssdk.GetItemOutput, error) {
	a.getInput = in
	return a.getOutput, a.getErr
}

func (a *repositoryAPI) PutItem(_ context.Context, in *awssdk.PutItemInput, _ ...func(*awssdk.Options)) (*awssdk.PutItemOutput, error) {
	a.putInput = in
	return a.putOutput, a.putErr
}

func (a *repositoryAPI) UpdateItem(_ context.Context, in *awssdk.UpdateItemInput, _ ...func(*awssdk.Options)) (*awssdk.UpdateItemOutput, error) {
	a.updateInput = in
	a.updateInputs = append(a.updateInputs, in)
	return a.updateOutput, a.updateErr
}

// Query は queryOutputs を呼び出し順に 1 ページずつ返す。
func (a *repositoryAPI) Query(_ context.Context, in *awssdk.QueryInput, _ ...func(*awssdk.Options)) (*awssdk.QueryOutput, error) {
	a.queryInputs = append(a.queryInputs, in)
	if a.queryErr != nil {
		return nil, a.queryErr
	}
	if len(a.queryOutputs) == 0 {
		return &awssdk.QueryOutput{}, nil
	}
	out := a.queryOutputs[0]
	a.queryOutputs = a.queryOutputs[1:]
	return out, nil
}
