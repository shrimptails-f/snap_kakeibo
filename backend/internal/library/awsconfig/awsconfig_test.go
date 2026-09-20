package awsconfig

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/library/stage"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// isolateSharedConfig は ~/.aws やプロファイルを読まないようにする。
func isolateSharedConfig(t *testing.T) {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "missing")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_CONFIG_FILE", missing)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", missing)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

func TestLoadLocalPointsAtEndpointWithDummyCredentials(t *testing.T) {
	isolateSharedConfig(t)
	for _, st := range []stage.Stage{stage.Local, stage.CI} {
		t.Setenv(stage.EnvKey, st.String())
		t.Setenv(EnvEndpointURL, "http://floci:4566")
		t.Setenv(EnvRegion, "ap-northeast-2")

		cfg, err := Load(context.Background(), oswrapper.New())
		if err != nil {
			t.Fatalf("%s: %v", st, err)
		}
		if aws.ToString(cfg.BaseEndpoint) != "http://floci:4566" || cfg.Region != "ap-northeast-2" {
			t.Fatalf("%s: endpoint=%q region=%q", st, aws.ToString(cfg.BaseEndpoint), cfg.Region)
		}
		creds, err := cfg.Credentials.Retrieve(context.Background())
		if err != nil || creds.AccessKeyID != localCredential || creds.SecretAccessKey != localCredential {
			t.Fatalf("%s: creds=%+v err=%v", st, creds, err)
		}
	}
}

func TestLoadLocalRequiresEndpointAndRegion(t *testing.T) {
	isolateSharedConfig(t)
	t.Setenv(stage.EnvKey, "local")
	t.Setenv(EnvRegion, "ap-northeast-2")

	t.Setenv(EnvEndpointURL, "")
	if _, err := Load(context.Background(), oswrapper.New()); err == nil || !strings.Contains(err.Error(), EnvEndpointURL) {
		t.Fatalf("missing endpoint: err=%v", err)
	}

	t.Setenv(EnvEndpointURL, "http://floci:4566")
	t.Setenv(EnvRegion, "")
	if _, err := Load(context.Background(), oswrapper.New()); err == nil || !strings.Contains(err.Error(), EnvRegion) {
		t.Fatalf("missing region: err=%v", err)
	}
}

func TestLoadRejectsUnknownStage(t *testing.T) {
	isolateSharedConfig(t)
	t.Setenv(stage.EnvKey, "production")

	if _, err := Load(context.Background(), oswrapper.New()); err == nil {
		t.Fatal("unknown stage must be an error")
	}
}

func TestLoadUsesDefaultResolutionForAWSStagesAndUnset(t *testing.T) {
	isolateSharedConfig(t)
	t.Setenv(EnvEndpointURL, "")
	t.Setenv(EnvRegion, "us-west-2")

	for _, st := range []string{"dev", "stg", "prd", ""} {
		t.Setenv(stage.EnvKey, st)
		cfg, err := Load(context.Background(), oswrapper.New())
		if err != nil {
			t.Fatalf("STAGE=%q: %v", st, err)
		}
		if cfg.BaseEndpoint != nil || cfg.Region != "us-west-2" {
			t.Fatalf("STAGE=%q: endpoint=%v region=%q", st, cfg.BaseEndpoint, cfg.Region)
		}
	}
}
