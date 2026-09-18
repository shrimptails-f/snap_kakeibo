package main

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"snap_kakeibo/infra/common"
	"snap_kakeibo/infra/config"
)

type fakeStore struct {
	existing map[string]string
	puts     []ssm.PutParameterInput
}

func (f *fakeStore) GetParameter(_ context.Context, in *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	if _, ok := f.existing[*in.Name]; ok {
		return &ssm.GetParameterOutput{}, nil
	}
	return nil, &types.ParameterNotFound{}
}

func (f *fakeStore) PutParameter(_ context.Context, in *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	f.puts = append(f.puts, *in)
	f.existing[*in.Name] = *in.Value
	return &ssm.PutParameterOutput{}, nil
}

func TestEnsureAllCreatesOnlyMissing(t *testing.T) {
	cfg := config.Dev()
	store := &fakeStore{existing: map[string]string{
		cfg.Parameters.OpenAIAPIKey: "sk-existing",
	}}

	if err := ensureAll(context.Background(), store, parametersFor(cfg)); err != nil {
		t.Fatal(err)
	}

	// API key 以外の共通パラメータ + CI/CD パラメータ + 関数ごとのイメージタグ
	if want := 2 + 4 + len(cfg.Functions); len(store.puts) != want {
		t.Fatalf("expected %d puts, got %d", want, len(store.puts))
	}
	for _, f := range cfg.Functions {
		if got := store.existing[f.ImageTagParameter]; got != common.UnsetParameterValue {
			t.Errorf("%s = %q, want %q", f.ImageTagParameter, got, common.UnsetParameterValue)
		}
	}
	if got := store.existing[cfg.Parameters.OpenAIAPIKey]; got != "sk-existing" {
		t.Errorf("existing value was overwritten: %q", got)
	}
	for _, name := range []string{
		cfg.CI.Parameters.GitHubConnectionARN,
		cfg.CI.Parameters.BackendLastSuccessfulCommit,
		cfg.CI.Parameters.FrontendLastSuccessfulCommit,
		cfg.CI.Parameters.InfraLastSuccessfulCommit,
	} {
		if got := store.existing[name]; got != common.UnsetParameterValue {
			t.Errorf("%s = %q, want %q", name, got, common.UnsetParameterValue)
		}
	}
	for _, p := range store.puts {
		if *p.Overwrite {
			t.Errorf("%s: Overwrite must be false", *p.Name)
		}
		if p.Tier != types.ParameterTierStandard {
			t.Errorf("%s: Tier = %v, want Standard", *p.Name, p.Tier)
		}
	}
	if len(store.existing[cfg.Parameters.PasswordPepper]) < 40 {
		t.Errorf("PasswordPepper looks too short: %q", store.existing[cfg.Parameters.PasswordPepper])
	}
}

func TestParametersForUsesOpenAIAPIKeyEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")

	byName := map[string]parameter{}
	for _, p := range parametersFor(config.Dev()) {
		byName[p.Name] = p
	}
	cfg := config.Dev()
	if got := byName[cfg.Parameters.OpenAIAPIKey].Value; got != "sk-test" {
		t.Errorf("OpenAIAPIKey = %q", got)
	}
}
