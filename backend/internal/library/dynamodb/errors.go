package dynamodb

import (
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
)

// トランザクション取り消し理由のコード(CancellationReason.Code)。
const (
	cancellationConditionalCheckFailed = "ConditionalCheckFailed"
	cancellationValidationError        = "ValidationError"
)

// IsConditionalCheckFailed は条件式が成立せず書き込みが行われなかったエラーなら true。
// 単体の PutItem / UpdateItem の ConditionalCheckFailedException と、トランザクション内のいずれかの項目が
// 条件不成立で取り消された TransactionCanceledException の両方を判定する。
func IsConditionalCheckFailed(err error) bool {
	var conditional *types.ConditionalCheckFailedException
	if errors.As(err, &conditional) {
		return true
	}
	if hasCancellationReason(err, cancellationConditionalCheckFailed) {
		return true
	}
	// 型付きの例外に変換されなかった場合(エンドポイントによって理由が本文にしか載らないことがある)の保険
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "ConditionalCheckFailedException":
		return true
	case "TransactionCanceledException":
		return strings.Contains(apiErr.ErrorMessage(), cancellationConditionalCheckFailed)
	}
	return false
}

// IsTransactionValidationFailed はトランザクション内のいずれかの項目が ValidationError
// (項目サイズ超過や式の不正など、リトライしても解決しない入力の問題)で取り消された場合に true。
func IsTransactionValidationFailed(err error) bool {
	return hasCancellationReason(err, cancellationValidationError)
}

func hasCancellationReason(err error, code string) bool {
	var canceled *types.TransactionCanceledException
	if !errors.As(err, &canceled) {
		return false
	}
	for _, reason := range canceled.CancellationReasons {
		if aws.ToString(reason.Code) == code {
			return true
		}
	}
	return false
}
