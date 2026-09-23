package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// newS3Proxy は /s3/<bucket>/<key> を Floci の /<bucket>/<key> へ中継する。
// ブラウザ(localhost:5173)からは別 origin になるので、preflight を含めて CORS を許可する。
// 本番では S3 バケットの CORS 設定が同じ役割を持つ。
func newS3Proxy(prefix string, endpoint *url.URL) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(endpoint)
			r.Out.URL.Path = strings.TrimPrefix(r.In.URL.Path, prefix)
			r.Out.URL.RawPath = strings.TrimPrefix(r.In.URL.RawPath, prefix)
			r.Out.Host = endpoint.Host
		},
		ModifyResponse: func(res *http.Response) error {
			setCORS(res.Header)
			return nil
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			setCORS(w.Header())
			w.WriteHeader(http.StatusNoContent)
			return
		}
		proxy.ServeHTTP(w, r)
	})
}

func setCORS(h http.Header) {
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "content-type")
	h.Set("Access-Control-Expose-Headers", "etag")
}
