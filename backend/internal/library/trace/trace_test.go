package trace

import (
	"context"
	"strings"
	"testing"
)

func TestNewRootAndChild(t *testing.T) {
	t.Parallel()
	root := NewRoot()
	if !root.Valid() {
		t.Fatalf("root is invalid: %+v", root)
	}
	if root.ParentSpanID != "" || !root.Sampled {
		t.Fatalf("root=%+v", root)
	}

	child := root.NewChild()
	if child.TraceID != root.TraceID || child.SpanID == root.SpanID || child.ParentSpanID != root.SpanID {
		t.Fatalf("child=%+v root=%+v", child, root)
	}

	if other := NewRoot(); other.TraceID == root.TraceID {
		t.Fatal("trace ids must differ")
	}
}

func TestInvalidChildFallsBackToRoot(t *testing.T) {
	t.Parallel()
	c := Context{}.NewChild()
	if !c.Valid() || c.ParentSpanID != "" {
		t.Fatalf("got %+v", c)
	}
}

func TestContextRoundTrip(t *testing.T) {
	t.Parallel()
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("empty ctx must not have trace")
	}
	if _, ok := FromContext(nil); ok { //nolint:staticcheck // nil ctx で panic しないことを確認
		t.Fatal("nil ctx must not have trace")
	}
	if _, ok := FromContext(ContextWith(context.Background(), Context{TraceID: "bad"})); ok {
		t.Fatal("invalid trace must be ignored")
	}

	want := NewRoot()
	got, ok := FromContext(ContextWith(context.Background(), want))
	if !ok || got != want {
		t.Fatalf("got %+v ok=%v want %+v", got, ok, want)
	}
}

func TestStart(t *testing.T) {
	t.Parallel()

	ctx, root := Start(context.Background())
	if got, ok := FromContext(ctx); !ok || got != root || root.ParentSpanID != "" {
		t.Fatalf("root: got %+v ok=%v", got, ok)
	}

	ctx2, child := Start(ctx)
	if child.TraceID != root.TraceID || child.ParentSpanID != root.SpanID {
		t.Fatalf("child=%+v root=%+v", child, root)
	}
	if got, _ := FromContext(ctx2); got != child {
		t.Fatalf("ctx2 has %+v want %+v", got, child)
	}
	if got, _ := FromContext(ctx); got != root {
		t.Fatal("parent ctx must be unchanged")
	}
}

func TestStartFrom(t *testing.T) {
	t.Parallel()
	remote := Context{TraceID: strings.Repeat("a", 32), SpanID: strings.Repeat("b", 16), Sampled: true}

	_, child := StartFrom(context.Background(), remote)
	if child.TraceID != remote.TraceID || child.ParentSpanID != remote.SpanID {
		t.Fatalf("child=%+v", child)
	}

	local, _ := Start(context.Background())
	_, fallback := StartFrom(local, Context{})
	if l, _ := FromContext(local); fallback.TraceID != l.TraceID || fallback.ParentSpanID != l.SpanID {
		t.Fatalf("invalid remote should fall back to local parent: %+v", fallback)
	}
}

func TestTraceparent(t *testing.T) {
	t.Parallel()
	tc := Context{TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SpanID: "00f067aa0ba902b7", Sampled: true}
	header := tc.Traceparent()
	if header != "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" {
		t.Fatalf("header=%s", header)
	}

	got, ok := ParseTraceparent(header)
	if !ok || got != tc {
		t.Fatalf("got %+v ok=%v", got, ok)
	}

	got, ok = ParseTraceparent("00-4BF92F3577B34DA6A3CE929D0E0E4736-00F067AA0BA902B7-00")
	if !ok || got.Sampled || got.TraceID != tc.TraceID {
		t.Fatalf("got %+v ok=%v", got, ok)
	}

	for _, bad := range []string{
		"",
		"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01",
		"00-xyz-00f067aa0ba902b7-01",
		"garbage",
	} {
		if _, ok := ParseTraceparent(bad); ok {
			t.Fatalf("%q should be invalid", bad)
		}
	}
	if (Context{}).Traceparent() != "" {
		t.Fatal("invalid context must produce empty header")
	}
}

func TestXRayTraceHeader(t *testing.T) {
	t.Parallel()
	tc := Context{TraceID: "5759e988bd862e3fe1be46a994272793", SpanID: "53995c3f42cd8ad8", Sampled: true}
	header := tc.XRayTraceHeader()
	if header != "Root=1-5759e988-bd862e3fe1be46a994272793;Parent=53995c3f42cd8ad8;Sampled=1" {
		t.Fatalf("header=%s", header)
	}

	got, ok := ParseXRayTraceHeader(header)
	if !ok || got != tc {
		t.Fatalf("got %+v ok=%v", got, ok)
	}

	// Lambda の入口では Parent が無いことがある
	got, ok = ParseXRayTraceHeader("Root=1-5759e988-bd862e3fe1be46a994272793;Sampled=0")
	if !ok || got.TraceID != tc.TraceID || !isHex(got.SpanID, spanIDLen) || got.Sampled {
		t.Fatalf("got %+v ok=%v", got, ok)
	}

	// 順不同・空白入りも許容
	got, ok = ParseXRayTraceHeader("Sampled=1; Parent=53995c3f42cd8ad8; Root=1-5759e988-bd862e3fe1be46a994272793")
	if !ok || got != tc {
		t.Fatalf("got %+v ok=%v", got, ok)
	}

	for _, bad := range []string{"", "Root=2-5759e988-bd862e3fe1be46a994272793", "Root=1-xyz", "Parent=53995c3f42cd8ad8"} {
		if _, ok := ParseXRayTraceHeader(bad); ok {
			t.Fatalf("%q should be invalid", bad)
		}
	}
}

func TestRootTraceIDStartsWithTimestamp(t *testing.T) {
	t.Parallel()
	root := NewRoot()
	if _, ok := ParseXRayTraceHeader(root.XRayTraceHeader()); !ok {
		t.Fatalf("root must round-trip through X-Ray format: %s", root.XRayTraceHeader())
	}
}
