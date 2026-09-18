// hello は疎通確認用の Lambda。GET /api/hello に固定の JSON を返す。
package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type response struct {
	Message string `json:"message"`
	Path    string `json:"path"`
	Time    string `json:"time"`
}

func handler(_ context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	body, err := json.Marshal(response{
		Message: "hello from snap_kakeibo",
		Path:    req.RawPath,
		Time:    time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	return events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Headers:    map[string]string{"content-type": "application/json"},
		Body:       string(body),
	}, nil
}

func main() {
	lambda.Start(handler)
}
