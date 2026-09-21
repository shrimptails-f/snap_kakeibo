// Package apigateway は API Gateway(HTTP API v2)へ返すレスポンスの組み立てを提供する。
// cmd/<lambda> の handler が application の結果やエラーを HTTP に変換するときだけ使い、application からは参照しない。
package apigateway

import (
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
)

// JSON は v を JSON にして status で返す。v を JSON にできなければ 500 と error を返し、lambdawrap がログに残す。
func JSON(status int, v any) (events.APIGatewayV2HTTPResponse, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    map[string]string{"content-type": "application/json"},
		Body:       string(body),
	}, nil
}

// Error は {"error": message} を status で返す。message は利用者に見せてよい短い文にし、内部エラーの内容は含めない。
func Error(status int, message string) (events.APIGatewayV2HTTPResponse, error) {
	return JSON(status, map[string]string{"error": message})
}
