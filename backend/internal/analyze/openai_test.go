package analyze

import "testing"

func TestFailureFromHTTPExtractsSafeErrorMetadata(t *testing.T) {
	t.Parallel()
	f := failureFromHTTP(404, []byte(`{"error":{"message":"model missing","type":"invalid_request_error","code":"model_not_found"}}`))
	if f.HTTPStatus != 404 || f.ProviderType != "invalid_request_error" || f.ProviderCode != "model_not_found" || f.ProviderMessage != "model missing" {
		t.Fatalf("failure=%+v", f)
	}
}

func TestReceiptSchemaRequiresDateOnly(t *testing.T) {
	t.Parallel()
	properties := receiptSchema()["properties"].(map[string]any)
	purchasedAt := properties["purchased_at"].(map[string]any)
	variants := purchasedAt["anyOf"].([]any)
	date := variants[0].(map[string]any)
	if got := date["pattern"]; got != `^\d{4}-\d{2}-\d{2}$` {
		t.Fatalf("purchased_at pattern = %v", got)
	}
}
