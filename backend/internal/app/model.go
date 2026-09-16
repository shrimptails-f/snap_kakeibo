package app

import (
	"crypto/rand"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

func NewID() (string, error) {
	id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func UserPK(userID string) string { return "USER#" + userID }

func UploadSK(uploadID string) string { return "UPLOAD#" + uploadID }

func BillingSK(billingID string) string { return "BILLING#" + billingID }

func DetailPK(userID, billingID string) string {
	return "USER#" + userID + "#BILLING#" + billingID
}

func DetailSK(detailID string) string { return "DETAIL#" + detailID }

func MonthSK(month string) string { return "MONTH#" + month }

func UploadMonthPK(userID, month string) string {
	return "USER#" + userID + "#MONTH#" + month
}

func UploadMonthSK(createdAt, uploadID string) string {
	return "UPLOAD_CREATED_AT#" + createdAt + "#" + uploadID
}

func DetailMonthSK(amount int64, purchasedAt, detailID string) string {
	desc := int64(math.MaxInt32) - amount
	if desc < 0 {
		desc = 0
	}
	return fmt.Sprintf("DETAIL_AMOUNT#%010d#%s#%s", desc, purchasedAt, detailID)
}

func YearMonth(t time.Time) string { return t.UTC().Format("2006-01") }

func YearMonthFromDate(date string) string {
	if len(date) >= 7 {
		return date[:7]
	}
	return YearMonth(time.Now())
}

func SplitJobTag(tag string) (userID, uploadID, attempt string, ok bool) {
	parts := strings.Split(tag, "#")
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}
