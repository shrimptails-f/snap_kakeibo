package stage

import (
	"testing"

	"snap_kakeibo/backend/internal/library/oswrapper"
)

func TestParse(t *testing.T) {
	t.Parallel()
	valid := map[string]Stage{"local": Local, "ci": CI, "dev": Dev, "stg": Stg, "prd": Prd, " PRD ": Prd}
	for in, want := range valid {
		if got, err := Parse(in); err != nil || got != want {
			t.Fatalf("Parse(%q)=%q err=%v want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "prod", "production", "staging"} {
		if got, err := Parse(in); err == nil {
			t.Fatalf("Parse(%q)=%q want error", in, got)
		}
	}
}

func TestIsLocal(t *testing.T) {
	t.Parallel()
	for s, want := range map[Stage]bool{Local: true, CI: true, Dev: false, Stg: false, Prd: false} {
		if got := s.IsLocal(); got != want {
			t.Fatalf("%s.IsLocal()=%v want %v", s, got, want)
		}
	}
}

func TestFromEnv(t *testing.T) {
	osw := oswrapper.New()

	t.Setenv(EnvKey, "ci")
	if got, err := FromEnv(osw); err != nil || got != CI {
		t.Fatalf("got %q err=%v", got, err)
	}

	t.Setenv(EnvKey, "")
	if got, err := FromEnv(osw); err == nil {
		t.Fatalf("unset must be an error, got %q", got)
	}

	t.Setenv(EnvKey, "moon")
	if got, err := FromEnv(osw); err == nil {
		t.Fatalf("unknown must be an error, got %q", got)
	}
}
