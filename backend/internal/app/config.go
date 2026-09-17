package app

import "os"

const FixedUserID = "01JUSER0000000000000000000"

type Config struct {
	UsersTable               string
	MonthlySummariesTable    string
	UploadHistoriesTable     string
	BillingsTable            string
	BillingDetailsTable      string
	ReceiptBucket            string
	AnalyzeQueueURL          string
	OpenAIAPIKeyParameter    string
	OpenAIModelParameter     string
	OpenAIReasoningParameter string
	ImageMaxEdge             string
}

func LoadConfig() Config {
	return Config{
		UsersTable:               os.Getenv("USERS_TABLE"),
		MonthlySummariesTable:    os.Getenv("MONTHLY_SUMMARIES_TABLE"),
		UploadHistoriesTable:     os.Getenv("UPLOAD_HISTORIES_TABLE"),
		BillingsTable:            os.Getenv("BILLINGS_TABLE"),
		BillingDetailsTable:      os.Getenv("BILLING_DETAILS_TABLE"),
		ReceiptBucket:            os.Getenv("RECEIPT_BUCKET"),
		AnalyzeQueueURL:          os.Getenv("ANALYZE_QUEUE_URL"),
		OpenAIAPIKeyParameter:    os.Getenv("SSM_OPENAI_API_KEY"),
		OpenAIModelParameter:     os.Getenv("SSM_OPENAI_MODEL"),
		OpenAIReasoningParameter: os.Getenv("SSM_OPENAI_REASONING_EFFORT"),
		ImageMaxEdge:             os.Getenv("IMAGE_MAX_EDGE"),
	}
}
