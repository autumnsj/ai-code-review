package notifier

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// 钉钉群机器人（支持加签）
type dingTalkNotifier struct {
	webhook string
	secret  string
	client  *http.Client
}

func (n *dingTalkNotifier) Send(ctx context.Context, title, markdown string) error {
	webhook := n.webhook
	if n.secret != "" {
		ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
		stringToSign := ts + "\n" + n.secret
		mac := hmac.New(sha256.New, []byte(n.secret))
		mac.Write([]byte(stringToSign))
		sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		webhook += "&timestamp=" + ts + "&sign=" + url.QueryEscape(sign)
	}
	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  markdown,
		},
	}
	return postJSON(ctx, n.client, webhook, payload)
}
