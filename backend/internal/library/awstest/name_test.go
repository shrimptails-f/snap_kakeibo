package awstest

import (
	"regexp"
	"testing"
)

func TestResourceName(t *testing.T) {
	t.Parallel()
	first := ResourceName("s3-roundtrip")
	second := ResourceName("s3-roundtrip")
	pattern := regexp.MustCompile(`^s3-roundtrip-[0-9a-z]{16}$`)
	if !pattern.MatchString(first) || !pattern.MatchString(second) {
		t.Fatalf("ResourceName() = %q, %q", first, second)
	}
	if first == second {
		t.Fatalf("ResourceName() returned duplicate %q", first)
	}
}
