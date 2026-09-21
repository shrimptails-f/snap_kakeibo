package apigateway

import (
	"testing"
)

func TestJSONEncodesBodyWithContentType(t *testing.T) {
	t.Parallel()
	resp, err := JSON(201, map[string]int{"n": 1})
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if resp.StatusCode != 201 || resp.Body != `{"n":1}` || resp.Headers["content-type"] != "application/json" {
		t.Errorf("JSON() = %+v", resp)
	}
}

func TestJSONReturns500WhenBodyCannotBeEncoded(t *testing.T) {
	t.Parallel()
	resp, err := JSON(200, make(chan int))
	if err == nil || resp.StatusCode != 500 {
		t.Errorf("JSON(chan) = %+v, %v, want 500 and error", resp, err)
	}
}

func TestErrorWrapsMessage(t *testing.T) {
	t.Parallel()
	resp, err := Error(401, "unauthorized")
	if err != nil {
		t.Fatalf("Error() error = %v", err)
	}
	if resp.StatusCode != 401 || resp.Body != `{"error":"unauthorized"}` {
		t.Errorf("Error() = %+v", resp)
	}
}
