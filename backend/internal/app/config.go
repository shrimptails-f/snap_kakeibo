package app

import "os"

const FixedUserID = "01JUSER0000000000000000000"

type Config struct {
	UsersTable            string
	MonthlySummariesTable string
	UploadHistoriesTable  string
	BillingsTable         string
	BillingDetailsTable   string
	ReceiptBucket         string
	TextractTopicARN      string
	TextractRoleARN       string
}

func LoadConfig() Config {
	return Config{
		UsersTable:            os.Getenv("USERS_TABLE"),
		MonthlySummariesTable: os.Getenv("MONTHLY_SUMMARIES_TABLE"),
		UploadHistoriesTable:  os.Getenv("UPLOAD_HISTORIES_TABLE"),
		BillingsTable:         os.Getenv("BILLINGS_TABLE"),
		BillingDetailsTable:   os.Getenv("BILLING_DETAILS_TABLE"),
		ReceiptBucket:         os.Getenv("RECEIPT_BUCKET"),
		TextractTopicARN:      os.Getenv("TEXTRACT_TOPIC_ARN"),
		TextractRoleARN:       os.Getenv("TEXTRACT_ROLE_ARN"),
	}
}
