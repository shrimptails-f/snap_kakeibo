package analyze

import "testing"

func TestFailureFromHTTPExtractsSafeErrorMetadata(t *testing.T) {
	f := failureFromHTTP(404, []byte(`{"error":{"message":"model missing","type":"invalid_request_error","code":"model_not_found"}}`))
	if f.HTTPStatus != 404 || f.ProviderType != "invalid_request_error" || f.ProviderCode != "model_not_found" || f.ProviderMessage != "model missing" {
		t.Fatalf("failure=%+v", f)
	}
}
