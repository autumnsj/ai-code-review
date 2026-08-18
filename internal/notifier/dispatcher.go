package notifier

import (
	"context"

	"go.uber.org/zap"

	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/store"
)

const settingsKey = "notifiers"

// Dispatcher 读取已配置的通知渠道并发送。
type Dispatcher struct {
	store   *store.Store
	log     *zap.Logger
	baseURL string
}

func NewDispatcher(st *store.Store, log *zap.Logger, baseURL string) *Dispatcher {
	return &Dispatcher{store: st, log: log, baseURL: baseURL}
}

func (d *Dispatcher) channels(ctx context.Context) ([]Channel, error) {
	var chs []Channel
	if err := d.store.GetSetting(ctx, settingsKey, &chs); err != nil {
		if store.IsSettingNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return chs, nil
}

// NotifyReview 审查完成/失败后，根据配置推送到启用的渠道。
func (d *Dispatcher) NotifyReview(ctx context.Context, r *domain.Review, findings []*domain.Finding) {
	chs, err := d.channels(ctx)
	if err != nil {
		d.log.Error("load notifier channels", zap.Error(err))
		return
	}
	reportURL := d.baseURL + "/reports/" + r.PublicToken
	md := BuildMarkdown(r, findings, reportURL)
	title := "代码审查报告 - " + r.RepoName

	for _, ch := range chs {
		if !ch.Enabled || ch.WebhookURL == "" {
			continue
		}
		n, err := New(ch)
		if err != nil {
			d.log.Warn("create notifier", zap.String("type", ch.Type), zap.Error(err))
			continue
		}
		if err := n.Send(ctx, title, md); err != nil {
			d.log.Warn("send notification", zap.String("type", ch.Type), zap.Error(err))
		}
	}
}

// NotifyAuthorReview 按作者逐条发送通知：每位参与者收到自己的评分、问题与专属公开报告链接。
// 当该次审查没有拆分出作者报告时（单作者/浅克隆等），回退到整体报告通知。
func (d *Dispatcher) NotifyAuthorReview(ctx context.Context, r *domain.Review) {
	reports, err := d.store.ListAuthorReportsByReview(ctx, r.ID)
	if err != nil {
		d.log.Warn("notify: load author reports", zap.Error(err))
	}
	if len(reports) == 0 {
		// 没有作者拆分，退回到整体通知。
		findings, ferr := d.store.ListFindings(ctx, r.ID)
		if ferr != nil {
			d.log.Warn("notify: load findings", zap.Error(ferr))
		}
		d.NotifyReview(ctx, r, findings)
		return
	}

	chs, err := d.channels(ctx)
	if err != nil {
		d.log.Error("load notifier channels", zap.Error(err))
		return
	}
	enabled := make([]Channel, 0, len(chs))
	for _, ch := range chs {
		if ch.Enabled && ch.WebhookURL != "" {
			enabled = append(enabled, ch)
		}
	}
	if len(enabled) == 0 {
		return
	}

	for _, ar := range reports {
		findings, ferr := d.store.ListFindingsByAuthor(ctx, r.ID, ar.Author)
		if ferr != nil {
			d.log.Warn("notify: load author findings", zap.String("author", ar.Author), zap.Error(ferr))
		}
		reportURL := d.baseURL + "/author-reports/" + ar.PublicToken
		md := BuildAuthorMarkdown(r, ar, findings, reportURL)
		title := "代码审查报告 - " + r.RepoName + " - " + ar.Author
		d.sendToChannels(ctx, enabled, title, md)
	}
}

func (d *Dispatcher) sendToChannels(ctx context.Context, chs []Channel, title, md string) {
	for _, ch := range chs {
		n, err := New(ch)
		if err != nil {
			d.log.Warn("create notifier", zap.String("type", ch.Type), zap.Error(err))
			continue
		}
		if err := n.Send(ctx, title, md); err != nil {
			d.log.Warn("send notification", zap.String("type", ch.Type), zap.Error(err))
		}
	}
}

// GetChannels 供 UI 读取。
func (d *Dispatcher) GetChannels(ctx context.Context) ([]Channel, error) {
	return d.channels(ctx)
}

// SaveChannels 供 UI 保存。
func (d *Dispatcher) SaveChannels(ctx context.Context, chs []Channel) error {
	if chs == nil {
		chs = []Channel{}
	}
	return d.store.SetSetting(ctx, settingsKey, chs)
}
