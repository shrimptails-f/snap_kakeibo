package oswrappertest

import "testing"

func TestMock(t *testing.T) {
	t.Parallel()
	mock := New(map[string]string{"STAGE": "ci"})
	mock.Files["config.json"] = "{}"

	if got, err := mock.GetEnv("STAGE"); err != nil || got != "ci" {
		t.Fatalf("GetEnv() = %q, %v", got, err)
	}
	if _, err := mock.GetEnv("MISSING"); err == nil {
		t.Fatal("GetEnv() error = nil, want missing error")
	}
	if got, err := mock.ReadFile("config.json"); err != nil || got != "{}" {
		t.Fatalf("ReadFile() = %q, %v", got, err)
	}
	if _, err := mock.ReadFile("missing.json"); err == nil {
		t.Fatal("ReadFile() error = nil, want missing error")
	}
}
