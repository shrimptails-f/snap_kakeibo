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

// JobTagSep は Textract の JobTag / ClientRequestToken の区切り文字。
// どちらも許可される文字が英数字と -_ 程度に限られるため "#" は使えない。
const JobTagSep = "_"

func JobTag(userID, uploadID, attempt string) string {
	return userID + JobTagSep + uploadID + JobTagSep + attempt
}

func SplitJobTag(tag string) (userID, uploadID, attempt string, ok bool) {
	parts := strings.Split(tag, JobTagSep)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}
