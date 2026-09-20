package di

import (
	"testing"

	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/library/retry"
	libs3 "snap_kakeibo/backend/internal/library/s3"
	libsqs "snap_kakeibo/backend/internal/library/sqs"
	libssm "snap_kakeibo/backend/internal/library/ssm"
	"snap_kakeibo/backend/internal/library/timewrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestNewContainerProvidesCommonLambdaDependencies(t *testing.T) {
	t.Parallel()
	osw := oswrapper.New()
	log := logger.NewNop()
	container, err := NewContainer(aws.Config{Region: "ap-northeast-1"}, osw, log)
	if err != nil {
		t.Fatalf("NewContainer() error = %v", err)
	}
	if err := container.Invoke(func(ddb *libdynamodb.Client, ssmClient *libssm.Client, s3Client *libs3.Client, sqsClient *libsqs.Client, retrier *retry.Retrier, clock timewrapper.Interface, resolvedOSW oswrapper.Interface, resolvedLog logger.Interface) {
		if ddb == nil || ssmClient == nil || s3Client == nil || sqsClient == nil || retrier == nil || clock == nil || resolvedOSW != osw || resolvedLog != log {
			t.Fatal("common dependency is nil")
		}
	}); err != nil {
		t.Fatalf("resolve common dependencies: %v", err)
	}
}
