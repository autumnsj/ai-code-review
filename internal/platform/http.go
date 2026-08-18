package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func itoa(i int) string { return strconv.Itoa(i) }

// doRequest 发起带超时的 HTTP 请求并返回 body。header 设置鉴权等头。
// POST/PUT 且 body 非空时自动设置 Content-Type: application/json。
func doRequest(ctx context.Context, method, rawURL string, header http.Header, body []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rdr)
	if err != nil {
		return nil, err
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	if len(body) > 0 && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 单页上限 10MB
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

// httpGet 发起带超时的 GET 请求并返回 body。header 设置鉴权等头。
func httpGet(ctx context.Context, rawURL string, header http.Header) ([]byte, error) {
	return doRequest(ctx, http.MethodGet, rawURL, header, nil)
}

// httpPost 发起带超时的 POST JSON 请求并返回 body。
func httpPost(ctx context.Context, rawURL string, header http.Header, body []byte) ([]byte, error) {
	return doRequest(ctx, http.MethodPost, rawURL, header, body)
}

// httpDelete 发起带超时的 DELETE 请求。
func httpDelete(ctx context.Context, rawURL string, header http.Header) error {
	_, err := doRequest(ctx, http.MethodDelete, rawURL, header, nil)
	return err
}

// decodeJSON 解析 JSON。
func decodeJSON(body []byte, v any) error {
	return json.Unmarshal(body, v)
}

// mustJSON 序列化请求体（输入均为受控数据，失败即编程错误，panic）。
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// setQuery 给 URL 追加 query 参数。
func setQuery(rawURL string, params map[string]string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func trimSlash(s string) string { return strings.TrimRight(strings.TrimSpace(s), "/") }
