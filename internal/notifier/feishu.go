package notifier

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"time"
)

// 飞书群机器人（支持加签）
type feiShuNotifier struct {
	webhook string
	secret  string
	client  *http.Client
}

func (n *feiShuNotifier) Send(ctx context.Context, title, markdown string) error {
	webhook := n.webhook
	body := map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"header": map[string]any{
				"title":    map[string]string{"tag": "plain_text", "content": title},
				"template": "blue",
			},
			"elements": []map[string]any{
				{
					"tag":     "markdown",
					"content": markdown,
				},
			},
		},
	}
	if n.secret != "" {
		ts := time.Now().Unix()
		stringToSign := strconv.FormatInt(ts, 10) + "\n" + n.secret
		mac := hmac.New(sha256.New, []byte(n.secret))
		mac.Write([]byte(stringToSign))
		sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		body["timestamp"] = strconv.FormatInt(ts, 10)
		body["sign"] = sign
	}
	return postJSON(ctx, n.client, webhook, body)
}
