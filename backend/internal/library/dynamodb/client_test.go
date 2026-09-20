package dynamodb

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func TestTableBindsNameToRequest(t *testing.T) {
	api := &fakeAPI{}
	table := NewWithAPI(api).Table("scenario-users-123")
	if _, err := table.GetItem(context.Background(), &awssdk.GetItemInput{}); err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if got := aws.ToString(api.get.TableName); got != table.Name() {
		t.Errorf("table name = %q, want %q", got, table.Name())
	}
	if _, err := table.GetItem(context.Background(), &awssdk.GetItemInput{TableName: aws.String("another-table")}); err == nil {
		t.Error("GetItem() accepted a different table name")
	}
}

type fakeAPI struct{ get *awssdk.GetItemInput }

func (f *fakeAPI) GetItem(_ context.Context, in *awssdk.GetItemInput, _ ...func(*awssdk.Options)) (*awssdk.GetItemOutput, error) {
	f.get = in
	return &awssdk.GetItemOutput{}, nil
}

func (f *fakeAPI) PutItem(context.Context, *awssdk.PutItemInput, ...func(*awssdk.Options)) (*awssdk.PutItemOutput, error) {
	return &awssdk.PutItemOutput{}, nil
}
