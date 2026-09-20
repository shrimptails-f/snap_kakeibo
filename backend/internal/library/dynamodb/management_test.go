package dynamodb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// fakeManagerAPI は CREATING → ACTIVE、DELETING → 消滅 の遷移を DescribeTable の呼び出し回数で再現する。
type fakeManagerAPI struct {
	created   *awssdk.CreateTableInput
	deleted   []string
	status    types.TableStatus
	describes int
	// exists が false なら DescribeTable / DeleteTable は ResourceNotFound を返す
	exists bool
}

func (f *fakeManagerAPI) CreateTable(_ context.Context, in *awssdk.CreateTableInput, _ ...func(*awssdk.Options)) (*awssdk.CreateTableOutput, error) {
	f.created = in
	f.exists = true
	f.status = types.TableStatusCreating
	return &awssdk.CreateTableOutput{}, nil
}

func (f *fakeManagerAPI) DeleteTable(_ context.Context, in *awssdk.DeleteTableInput, _ ...func(*awssdk.Options)) (*awssdk.DeleteTableOutput, error) {
	if !f.exists {
		return nil, &types.ResourceNotFoundException{Message: aws.String("not found")}
	}
	f.deleted = append(f.deleted, aws.ToString(in.TableName))
	f.status = types.TableStatusDeleting
	return &awssdk.DeleteTableOutput{}, nil
}

func (f *fakeManagerAPI) DescribeTable(_ context.Context, in *awssdk.DescribeTableInput, _ ...func(*awssdk.Options)) (*awssdk.DescribeTableOutput, error) {
	if !f.exists {
		return nil, &types.ResourceNotFoundException{Message: aws.String("not found")}
	}
	f.describes++
	// 2 回目の問い合わせで遷移が終わる
	if f.describes >= 2 {
		switch f.status {
		case types.TableStatusCreating:
			f.status = types.TableStatusActive
		case types.TableStatusDeleting:
			f.exists = false
			return nil, &types.ResourceNotFoundException{Message: aws.String("not found")}
		}
	}
	return &awssdk.DescribeTableOutput{Table: &types.TableDescription{TableName: in.TableName, TableStatus: f.status}}, nil
}

func TestManagerCreateTableWaitsForActive(t *testing.T) {
	api := &fakeManagerAPI{}
	manager := NewManagerWithAPI(api).WithWaitTimeout(10 * time.Second)
	cleanup, err := manager.CreateTable(context.Background(), "test-users-1", UsersSchema)
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	if api.created == nil || aws.ToString(api.created.TableName) != "test-users-1" {
		t.Fatalf("created = %+v", api.created)
	}
	if api.status != types.TableStatusActive || api.describes < 2 {
		t.Errorf("returned before ACTIVE: status=%s describes=%d", api.status, api.describes)
	}
	exists, err := manager.TableExists(context.Background(), "test-users-1")
	if err != nil || !exists {
		t.Errorf("TableExists = %v, %v", exists, err)
	}

	// 返された Cleanup で削除でき、二度呼んでもエラーにならない
	for range 2 {
		if err := cleanup(context.Background()); err != nil {
			t.Fatalf("cleanup: %v", err)
		}
	}
	if len(api.deleted) != 1 || api.deleted[0] != "test-users-1" {
		t.Errorf("deleted = %v", api.deleted)
	}
	if exists, err := manager.TableExists(context.Background(), "test-users-1"); err != nil || exists {
		t.Errorf("after cleanup: TableExists = %v, %v", exists, err)
	}
}

func TestManagerCreateTableRejectsBadInput(t *testing.T) {
	api := &fakeManagerAPI{}
	manager := NewManagerWithAPI(api)
	if _, err := manager.CreateTable(context.Background(), "x", UsersSchema); err == nil {
		t.Error("short name accepted")
	}
	if _, err := manager.CreateTable(context.Background(), "test-users-1", Schema{Name: "users"}); err == nil {
		t.Error("schema without partition key accepted")
	}
	if api.created != nil {
		t.Error("CreateTable was called with invalid input")
	}
}

func TestManagerDeleteTableWaitsForRemoval(t *testing.T) {
	api := &fakeManagerAPI{exists: true, status: types.TableStatusActive}
	manager := NewManagerWithAPI(api).WithWaitTimeout(10 * time.Second)
	if err := manager.DeleteTable(context.Background(), "test-users-1"); err != nil {
		t.Fatalf("DeleteTable: %v", err)
	}
	if len(api.deleted) != 1 || api.deleted[0] != "test-users-1" {
		t.Errorf("deleted = %v", api.deleted)
	}
	if api.exists {
		t.Error("returned while the table still exists")
	}
	exists, err := manager.TableExists(context.Background(), "test-users-1")
	if err != nil || exists {
		t.Errorf("TableExists = %v, %v", exists, err)
	}
}

func TestManagerDeleteTableIgnoresMissing(t *testing.T) {
	api := &fakeManagerAPI{}
	if err := NewManagerWithAPI(api).DeleteTable(context.Background(), "test-users-missing"); err != nil {
		t.Fatalf("DeleteTable on a missing table: %v", err)
	}
	if len(api.deleted) != 0 {
		t.Errorf("deleted = %v", api.deleted)
	}
}

func TestIsTableNotFound(t *testing.T) {
	if !IsTableNotFound(&types.ResourceNotFoundException{}) {
		t.Error("ResourceNotFoundException not detected")
	}
	if IsTableNotFound(errors.New("other")) {
		t.Error("unrelated error detected as not found")
	}
}
