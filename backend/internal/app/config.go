package app

import "os"

const FixedUserID = "01JUSER0000000000000000000"

type Config struct {
	UsersTable            string
	RefreshTokensTable    string
	MonthlySummariesTable string
	UploadHistoriesTable  string
	BillingsTable         string
	BillingDetailsTable   string
	ReceiptBucket         string
	AnalyzeQueueURL       string
	JWTSecretParameter    string
	JWTSecret             string
	OpenAIAPIKeyParameter string
	OpenAIModel           string
	OpenAIReasoningEffort string
	ImageMaxEdge          string
	Stage                 string // Stage は dev / stg / prd などの環境名。ログの environment に載せる。
	LogLevel              string // LogLevel は debug / info / warn / error。空なら info。
}

func LoadConfig() Config {
	return Config{
		UsersTable:            os.Getenv("USERS_TABLE"),
		RefreshTokensTable:    os.Getenv("REFRESH_TOKENS_TABLE"),
		MonthlySummariesTable: os.Getenv("MONTHLY_SUMMARIES_TABLE"),
		UploadHistoriesTable:  os.Getenv("UPLOAD_HISTORIES_TABLE"),
		BillingsTable:         os.Getenv("BILLINGS_TABLE"),
		BillingDetailsTable:   os.Getenv("BILLING_DETAILS_TABLE"),
		ReceiptBucket:         os.Getenv("RECEIPT_BUCKET"),
		AnalyzeQueueURL:       os.Getenv("ANALYZE_QUEUE_URL"),
		JWTSecretParameter:    os.Getenv("SSM_JWT_SECRET"),
		JWTSecret:             os.Getenv("JWT_SECRET"),
		OpenAIAPIKeyParameter: os.Getenv("SSM_OPENAI_API_KEY"),
		OpenAIModel:           os.Getenv("OPENAI_MODEL"),
		OpenAIReasoningEffort: os.Getenv("OPENAI_REASONING_EFFORT"),
		ImageMaxEdge:          os.Getenv("IMAGE_MAX_EDGE"),
		Stage:                 os.Getenv("STAGE"),
		LogLevel:              os.Getenv("LOG_LEVEL"),
	}
}
