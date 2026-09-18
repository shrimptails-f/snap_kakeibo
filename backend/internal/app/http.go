package app

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
)

func JSON(status int, v any) (events.APIGatewayV2HTTPResponse, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers: map[string]string{
			"content-type": "application/json",
		},
		Body: string(body),
	}, nil
}

func Error(status int, message string) (events.APIGatewayV2HTTPResponse, error) {
	return JSON(status, map[string]string{"error": message})
}

func Required(value, name string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}
