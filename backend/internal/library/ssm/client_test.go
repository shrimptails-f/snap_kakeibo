package ssm

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

func TestParameterBindsNameToRequest(t *testing.T) {
	api := &fakeAPI{}
	parameter := NewWithAPI(api).Parameter("/scenario/secret-123")
	value, err := parameter.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "secret" || aws.ToString(api.input.Name) != parameter.Name() || !aws.ToBool(api.input.WithDecryption) {
		t.Errorf("input = %#v, value = %q", api.input, value)
	}
}

type fakeAPI struct{ input *awssdk.GetParameterInput }

func (f *fakeAPI) GetParameter(_ context.Context, in *awssdk.GetParameterInput, _ ...func(*awssdk.Options)) (*awssdk.GetParameterOutput, error) {
	f.input = in
	return &awssdk.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String("secret")}}, nil
}
