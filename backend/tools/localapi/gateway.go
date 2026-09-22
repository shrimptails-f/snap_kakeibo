package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda/messages"
)

// route は HTTP のメソッド・パスと呼び出す Lambda の対応。
type route struct {
	method string
	// path は net/http の ServeMux と同じ {name} 形式。API Gateway のパスパラメータ名と揃える
	path     string
	function string
	// transform はレスポンス本文をブラウザへ返す前に書き換える。ローカル固有の差分(presigned URL の向き先など)に限って使う
	transform func(transformContext, []byte) ([]byte, error)
}

// transformContext は transform が使うローカル環境の値。
type transformContext struct {
	// endpoint は Lambda から見た Floci(http://floci:4566)
	endpoint string
	// publicS3URL はブラウザから見た S3 の向き先(http://localhost:8080/s3)
	publicS3URL string
}

var pathParamPattern = regexp.MustCompile(`\{([^}]+)\}`)

// maxBodyBytes は API Gateway のペイロード上限に合わせる。画像はここを通らず presigned URL で S3 へ行く。
const maxBodyBytes = 10 << 20

func newGatewayHandler(pool *functionPool, rt route, tc transformContext) http.Handler {
	var params []string
	for _, m := range pathParamPattern.FindAllStringSubmatch(rt.path, -1) {
		params = append(params, m[1])
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, `{"message":"Bad Request"}`)
			return
		}
		if len(body) > maxBodyBytes {
			writeJSON(w, http.StatusRequestEntityTooLarge, `{"message":"Request Entity Too Large"}`)
			return
		}
		pathParams := map[string]string{}
		for _, name := range params {
			pathParams[name] = r.PathValue(name)
		}
		payload, err := json.Marshal(toEvent(r, rt, pathParams, body))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, `{"message":"Internal Server Error"}`)
			return
		}
		out, err := pool.invoke(rt.function, payload)
		if err != nil {
			// API Gateway はハンドラのエラーを 500 の固定メッセージにする。原因は localapi のログにだけ出す
			var invokeErr *messages.InvokeResponse_Error
			if errors.As(err, &invokeErr) {
				log.Printf("[%s] handler error: %s: %s", rt.function, invokeErr.Type, invokeErr.Message)
				writeJSON(w, http.StatusInternalServerError, `{"message":"Internal Server Error"}`)
				return
			}
			log.Printf("[%s] invoke failed: %v", rt.function, err)
			writeJSON(w, http.StatusBadGateway, `{"message":"Bad Gateway"}`)
			return
		}
		var res events.APIGatewayV2HTTPResponse
		if err := json.Unmarshal(out, &res); err != nil {
			log.Printf("[%s] response is not APIGatewayV2HTTPResponse: %v", rt.function, err)
			writeJSON(w, http.StatusInternalServerError, `{"message":"Internal Server Error"}`)
			return
		}
		if rt.transform != nil && res.StatusCode/100 == 2 {
			transformed, err := rt.transform(tc, []byte(res.Body))
			if err != nil {
				log.Printf("[%s] transform response: %v", rt.function, err)
				writeJSON(w, http.StatusInternalServerError, `{"message":"Internal Server Error"}`)
				return
			}
			res.Body = string(transformed)
		}
		writeResponse(w, res)
	})
}

// toEvent は API Gateway HTTP API(payload v2.0)と同じ形のイベントを組み立てる。
// ヘッダ名は小文字、複数値は "," 連結、Cookie は headers から外して cookies に入る。
func toEvent(r *http.Request, rt route, pathParams map[string]string, body []byte) events.APIGatewayV2HTTPRequest {
	now := time.Now().UTC()
	headers := map[string]string{}
	for name, values := range r.Header {
		lower := strings.ToLower(name)
		if lower == "cookie" {
			continue
		}
		headers[lower] = strings.Join(values, ",")
	}
	var cookies []string
	for _, c := range r.Cookies() {
		cookies = append(cookies, c.String())
	}
	query := map[string]string{}
	for name, values := range r.URL.Query() {
		query[name] = strings.Join(values, ",")
	}
	sourceIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		sourceIP = host
	}
	if forwarded := r.Header.Get("x-forwarded-for"); forwarded != "" {
		// Vite の proxy 越しでもブラウザ側のアドレスを SourceIP にする(ログイン試行の制限が ip 単位のため)
		sourceIP = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	routeKey := rt.method + " " + rt.path
	return events.APIGatewayV2HTTPRequest{
		Version:               "2.0",
		RouteKey:              routeKey,
		RawPath:               r.URL.Path,
		RawQueryString:        r.URL.RawQuery,
		Cookies:               cookies,
		Headers:               headers,
		QueryStringParameters: query,
		PathParameters:        pathParams,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RouteKey:   routeKey,
			AccountID:  "000000000000",
			Stage:      "$default",
			RequestID:  requestID(),
			APIID:      "local",
			DomainName: r.Host,
			Time:       now.Format("02/Jan/2006:15:04:05 -0700"),
			TimeEpoch:  now.UnixMilli(),
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method:    r.Method,
				Path:      r.URL.Path,
				Protocol:  r.Proto,
				SourceIP:  sourceIP,
				UserAgent: r.UserAgent(),
			},
		},
		Body: string(body),
	}
}

// writeResponse は Lambda のレスポンスを HTTP に戻す。cookies は Set-Cookie に、statusCode 未指定は 200。
func writeResponse(w http.ResponseWriter, res events.APIGatewayV2HTTPResponse) {
	for name, value := range res.Headers {
		w.Header().Set(name, value)
	}
	for name, values := range res.MultiValueHeaders {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	for _, cookie := range res.Cookies {
		w.Header().Add("Set-Cookie", cookie)
	}
	body := []byte(res.Body)
	if res.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(res.Body)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, `{"message":"Internal Server Error"}`)
			return
		}
		body = decoded
	}
	status := res.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("content-length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// rewritePutURL は upload が返す presigned URL の向き先を、Lambda から見た Floci からブラウザから見た localapi の
// S3 中継に差し替える。Floci は署名の Host を検証しないので、URL の origin を変えても PUT は通る。
func rewritePutURL(tc transformContext, body []byte) ([]byte, error) {
	var res map[string]json.RawMessage
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	var putURL string
	if err := json.Unmarshal(res["put_url"], &putURL); err != nil {
		return nil, fmt.Errorf("put_url: %w", err)
	}
	if !strings.HasPrefix(putURL, tc.endpoint) {
		return nil, fmt.Errorf("put_url %q does not start with %s", putURL, tc.endpoint)
	}
	rewritten, err := json.Marshal(tc.publicS3URL + strings.TrimPrefix(putURL, tc.endpoint))
	if err != nil {
		return nil, err
	}
	res["put_url"] = rewritten
	return json.Marshal(res)
}
