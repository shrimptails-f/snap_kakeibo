package awsconfig

import (
	"context"
	"errors"
	"strings"
	"testing"

	"snap_kakeibo/backend/internal/library/oswrapper/oswrappertest"
	"snap_kakeibo/backend/internal/library/stage"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestLoadLocalPointsAtEndpointWithDummyCredentials(t *testing.T) {
	t.Parallel()
	for _, st := range []stage.Stage{stage.Local, stage.CI} {
		st := st
		t.Run(st.String(), func(t *testing.T) {
			t.Parallel()
			osw := oswrappertest.New(map[string]string{
				stage.EnvKey:   st.String(),
				EnvEndpointURL: "http://floci:4566",
				EnvRegion:      "ap-northeast-2",
			})
			cfg, err := Load(context.Background(), osw)
			if err != nil {
				t.Fatal(err)
			}
			if aws.ToString(cfg.BaseEndpoint) != "http://floci:4566" || cfg.Region != "ap-northeast-2" {
				t.Fatalf("endpoint=%q region=%q", aws.ToString(cfg.BaseEndpoint), cfg.Region)
			}
			creds, err := cfg.Credentials.Retrieve(context.Background())
			if err != nil || creds.AccessKeyID != localCredential || creds.SecretAccessKey != localCredential {
				t.Fatalf("creds=%+v err=%v", creds, err)
			}
		})
	}
}

func TestLoadLocalRequiresEndpointAndRegion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "endpoint", env: map[string]string{stage.EnvKey: "local", EnvRegion: "ap-northeast-2"}, want: EnvEndpointURL},
		{name: "region", env: map[string]string{stage.EnvKey: "local", EnvEndpointURL: "http://floci:4566"}, want: EnvRegion},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Load(context.Background(), oswrappertest.New(tt.env)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load() error = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestLoadRejectsUnknownStage(t *testing.T) {
	t.Parallel()
	osw := oswrappertest.New(map[string]string{stage.EnvKey: "production"})
	if _, err := Load(context.Background(), osw); err == nil {
		t.Fatal("unknown stage must be an error")
	}
}

func TestLoadUsesDefaultResolutionForAWSStagesAndUnset(t *testing.T) {
	t.Parallel()
	want := aws.Config{Region: "us-west-2"}
	for _, st := range []string{"dev", "stg", "prd", ""} {
		st := st
		t.Run(st, func(t *testing.T) {
			t.Parallel()
			calls := 0
			loader := func(context.Context) (aws.Config, error) {
				calls++
				return want, nil
			}
			cfg, err := load(context.Background(), oswrappertest.New(map[string]string{stage.EnvKey: st}), loader)
			if err != nil || cfg.Region != want.Region || calls != 1 {
				t.Fatalf("STAGE=%q: config=%+v calls=%d err=%v", st, cfg, calls, err)
			}
		})
	}
}

func TestLoadReturnsDefaultResolutionError(t *testing.T) {
	t.Parallel()
	want := errors.New("default config unavailable")
	_, err := load(context.Background(), oswrappertest.New(nil), func(context.Context) (aws.Config, error) {
		return aws.Config{}, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("load() error = %v, want %v", err, want)
	}
}
