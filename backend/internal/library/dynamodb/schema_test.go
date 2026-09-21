package dynamodb

import (
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestSchemaCreateTableInput(t *testing.T) {
	t.Parallel()
	in, err := AnalysisRequestsSchema.CreateTableInput("scenario-analysis-requests-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := aws.ToString(in.TableName); got != "scenario-analysis-requests-1" {
		t.Errorf("TableName = %q", got)
	}
	if in.BillingMode != types.BillingModePayPerRequest {
		t.Errorf("BillingMode = %q, want PAY_PER_REQUEST", in.BillingMode)
	}
	if got := keyNames(in.KeySchema); got != "PK/HASH,SK/RANGE" {
		t.Errorf("KeySchema = %s", got)
	}
	if len(in.GlobalSecondaryIndexes) != 1 {
		t.Fatalf("GSIs = %d, want 1", len(in.GlobalSecondaryIndexes))
	}
	gsi := in.GlobalSecondaryIndexes[0]
	if aws.ToString(gsi.IndexName) != AnalysisRequestMonthIndex || keyNames(gsi.KeySchema) != "GSI1PK/HASH,GSI1SK/RANGE" {
		t.Errorf("GSI = %s %s", aws.ToString(gsi.IndexName), keyNames(gsi.KeySchema))
	}
	if gsi.Projection == nil || gsi.Projection.ProjectionType != types.ProjectionTypeAll {
		t.Errorf("GSI projection = %+v, want ALL", gsi.Projection)
	}
	// キーに使う属性は文字列型で 1 回ずつ定義される
	want := map[string]bool{"PK": true, "SK": true, "GSI1PK": true, "GSI1SK": true}
	if len(in.AttributeDefinitions) != len(want) {
		t.Fatalf("AttributeDefinitions = %d, want %d", len(in.AttributeDefinitions), len(want))
	}
	for _, d := range in.AttributeDefinitions {
		if !want[aws.ToString(d.AttributeName)] || d.AttributeType != types.ScalarAttributeTypeS {
			t.Errorf("unexpected attribute definition %s/%s", aws.ToString(d.AttributeName), d.AttributeType)
		}
	}
}

func TestSchemaWithoutSortKey(t *testing.T) {
	t.Parallel()
	in, err := UsersSchema.CreateTableInput("test-users-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := keyNames(in.KeySchema); got != "PK/HASH" {
		t.Errorf("KeySchema = %s", got)
	}
	if len(in.AttributeDefinitions) != 1 || len(in.GlobalSecondaryIndexes) != 0 {
		t.Errorf("attrs=%d gsis=%d", len(in.AttributeDefinitions), len(in.GlobalSecondaryIndexes))
	}
}

func TestSchemaValidate(t *testing.T) {
	t.Parallel()
	for name, schema := range map[string]Schema{
		"no name":          {PartitionKey: "PK"},
		"no partition key": {Name: "users"},
		"gsi without name": {Name: "users", PartitionKey: "PK", GSIs: []GSI{{PartitionKey: "GSI1PK"}}},
	} {
		if err := schema.Validate(); err == nil {
			t.Errorf("%s: Validate() = nil, want error", name)
		}
	}
	for _, schema := range AllSchemas() {
		if err := schema.Validate(); err != nil {
			t.Errorf("%s: %v", schema.Name, err)
		}
	}
}

func TestRandomTableName(t *testing.T) {
	t.Parallel()
	pattern := regexp.MustCompile(`^scenario-users-[0-9a-f]{16}$`)
	seen := map[string]bool{}
	for range 100 {
		name := RandomTableName("scenario", UsersSchema)
		if !pattern.MatchString(name) {
			t.Fatalf("name %q does not match %s", name, pattern)
		}
		if err := ValidateTableName(name); err != nil {
			t.Fatal(err)
		}
		if seen[name] {
			t.Fatalf("duplicate name %q", name)
		}
		seen[name] = true
	}
	if got := RandomTableName("  ", ExpensesSchema); !regexp.MustCompile(`^test-expenses-`).MatchString(got) {
		t.Errorf("empty prefix: %q", got)
	}
}

func TestValidateTableName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"ab", "has space", "日本語", "bad/slash"} {
		if err := ValidateTableName(name); err == nil {
			t.Errorf("%q: want error", name)
		}
	}
	for _, name := range []string{"abc", "dev-snap-kakeibo-users", "a.b_c-1"} {
		if err := ValidateTableName(name); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
}

func keyNames(schema []types.KeySchemaElement) string {
	out := ""
	for i, k := range schema {
		if i > 0 {
			out += ","
		}
		out += aws.ToString(k.AttributeName) + "/" + string(k.KeyType)
	}
	return out
}
