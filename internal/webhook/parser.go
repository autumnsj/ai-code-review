package webhook

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/ai-code-review/aicr/internal/domain"
)

// Parser 把某个平台的原始 webhook 请求解析成统一事件。
type Parser interface {
	// Match 判断该请求是否来自此平台（通过 header）。
	Match(r *http.Request) bool
	// Parse 解析原始 body 与 header。不支持的事件返回 ErrIgnoredEvent。
	Parse(raw []byte, r *http.Request) (*domain.WebhookEvent, error)
}

var ErrIgnoredEvent = errors.New("ignored event")

// DefaultParsers 返回内置的平台解析器（顺序即匹配优先级）。
func DefaultParsers() []Parser {
	return []Parser{&githubParser{}, &gitlabParser{}, &giteeParser{}, &codingParser{}}
}

// verifyHMACHex 校验 hex 编码的 HMAC（sha1 或 sha256），常量时间比较。
func verifyHMACHex(alg, signature, secret string, body []byte) bool {
	if signature == "" || secret == "" {
		return false
	}
	var mac []byte
	switch alg {
	case "sha256":
		h := hmac.New(sha256.New, []byte(secret))
		h.Write(body)
		mac = h.Sum(nil)
	case "sha1":
		h := hmac.New(sha1.New, []byte(secret))
		h.Write(body)
		mac = h.Sum(nil)
	default:
		return false
	}
	expected := hex.EncodeToString(mac)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func constantTimeEqual(a, b string) bool { return hmac.Equal([]byte(a), []byte(b)) }
