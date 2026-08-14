package notifier

import (
	"context"
	"net/http"
)

// 企业微信群机器人
type weComNotifier struct {
	webhook string
	client  *http.Client
}

func (n *weComNotifier) Send(ctx context.Context, title, markdown string) error {
	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": markdown,
		},
	}
	return postJSON(ctx, n.client, n.webhook, payload)
}
