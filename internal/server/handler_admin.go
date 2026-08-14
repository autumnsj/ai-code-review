package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ai-code-review/aicr/internal/auth"
	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/notifier"
	"github.com/ai-code-review/aicr/internal/store"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// fetchModelsReq 拉取 OpenAI 兼容接口的模型列表。api_key 可选：
// 前端可传当前行已填未保存的 key；不传则后端会尝试从已保存的 llm 设置里按 base_url 匹配。
type fetchModelsReq struct {
	BaseURL string `json:"base_url" binding:"required"`
	APIKey  string `json:"api_key"`
}

type modelItem struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// POST /api/admin/settings/llm/fetch-models
func (s *Server) fetchModels(c *gin.Context) {
	var req fetchModelsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	key := strings.TrimSpace(req.APIKey)
	// 未传 key 时，尝试从已保存配置中匹配同 base_url 的 profile。
	if key == "" {
		var saved domain.LLMSettings
		if err := s.store.GetSetting(c.Request.Context(), "llm", &saved); err == nil {
			for _, p := range saved.Profiles {
				if strings.TrimRight(p.BaseURL, "/") == strings.TrimRight(req.BaseURL, "/") && p.APIKey != "" {
					key = p.APIKey
					break
				}
			}
		}
	}

	url := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/") + "/models"
	httpReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, url, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Base URL 无效: " + err.Error()})
		return
	}
	if key != "" {
		httpReq.Header.Set("Authorization", "Bearer "+key)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "无法连接模型服务: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 最多 2MB
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": "模型服务返回错误: " + resp.Status})
		return
	}
	var parsed struct {
		Data []modelItem `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "无法解析模型列表（接口返回非标准格式）"})
		return
	}
	models := make([]modelItem, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if m.ID == "" {
			continue
		}
		if m.Name == "" {
			m.Name = m.ID
		}
		models = append(models, m)
	}
	c.JSON(http.StatusOK, gin.H{"models": models})
}

// maskKey 脱敏 API key，保留首 3 尾 4。
func maskKey(k string) string {
	if len(k) <= 10 {
		return strings.Repeat("*", len(k))
	}
	return k[:3] + strings.Repeat("*", len(k)-7) + k[len(k)-4:]
}

// POST /api/admin/login
func (s *Server) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg, err := s.store.GetServerSettings(c.Request.Context())
	if err != nil {
		s.log.Error("get server settings", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器错误"})
		return
	}
	if !auth.CheckPassword(cfg.AdminPasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "密码错误"})
		return
	}
	token, err := s.jwt.Issue("admin")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "签发 token 失败"})
		return
	}
	c.JSON(http.StatusOK, loginResp{Token: token})
}

// POST /api/admin/change-password
func (s *Server) changePassword(c *gin.Context) {
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	cfg, err := s.store.GetServerSettings(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器错误"})
		return
	}
	if !auth.CheckPassword(cfg.AdminPasswordHash, req.OldPassword) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "原密码错误"})
		return
	}
	h, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "哈希失败"})
		return
	}
	cfg.AdminPasswordHash = h
	if err := s.store.SetSetting(ctx, "server", cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GET /api/admin/settings/llm
func (s *Server) getLLMSettings(c *gin.Context) {
	var cfg domain.LLMSettings
	if err := s.store.GetSetting(c.Request.Context(), "llm", &cfg); err != nil && !store.IsSettingNotFound(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取失败"})
		return
	}
	out := llmSettingsResp{DefaultID: cfg.DefaultID, Profiles: make([]llmProfileResp, 0, len(cfg.Profiles))}
	for _, p := range cfg.Profiles {
		out.Profiles = append(out.Profiles, llmProfileResp{
			ID:            p.ID,
			Name:          p.Name,
			BaseURL:       p.BaseURL,
			APIKeySet:     p.APIKey != "",
			APIKeyMasked:  maskKey(p.APIKey),
			Model:         p.Model,
			Temperature:   p.Temperature,
			MaxTokens:     p.MaxTokens,
			TimeoutSec:    p.TimeoutSec,
			ContextWindow: p.ContextWindow,
			Enabled:       p.Enabled,
		})
	}
	c.JSON(http.StatusOK, out)
}

// PUT /api/admin/settings/llm
func (s *Server) updateLLMSettings(c *gin.Context) {
	var req llmSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()

	// 读取旧值，用于保留「api_key 留空」的 profile 的已有 key。
	var old domain.LLMSettings
	_ = s.store.GetSetting(ctx, "llm", &old)
	oldKey := func(id string) string {
		for _, p := range old.Profiles {
			if p.ID == id {
				return p.APIKey
			}
		}
		return ""
	}

	profiles := make([]domain.LLMProfile, 0, len(req.Profiles))
	ids := make(map[string]bool, len(req.Profiles))
	for _, r := range req.Profiles {
		if r.ID == "" || r.BaseURL == "" || r.Model == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "每个模型都需填写 id、base_url、model"})
			return
		}
		if ids[r.ID] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "模型 id 重复: " + r.ID})
			return
		}
		ids[r.ID] = true
		key := r.APIKey
		if key == "" {
			key = oldKey(r.ID) // 留空不修改
		}
		timeout := r.TimeoutSec
		if timeout == 0 {
			timeout = 120
		}
		ctxWindow := r.ContextWindow
		if ctxWindow == 0 {
			ctxWindow = 64000
		}
		name := r.Name
		if name == "" {
			name = r.Model
		}
		profiles = append(profiles, domain.LLMProfile{
			ID:            r.ID,
			Name:          name,
			BaseURL:       r.BaseURL,
			APIKey:        key,
			Model:         r.Model,
			Temperature:   r.Temperature,
			MaxTokens:     r.MaxTokens,
			TimeoutSec:    timeout,
			ContextWindow: ctxWindow,
			Enabled:       r.Enabled,
		})
	}

	// default_id 必须指向提交列表中的某个 profile；有 profile 时必须指定默认。
	if len(profiles) > 0 {
		if req.DefaultID == "" || !ids[req.DefaultID] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请选择一个默认模型"})
			return
		}
	}

	if err := s.store.SetSetting(ctx, "llm", domain.LLMSettings{Profiles: profiles, DefaultID: req.DefaultID}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GET /api/admin/settings/notifications
func (s *Server) getNotifiers(c *gin.Context) {
	if s.notifiers == nil {
		c.JSON(http.StatusOK, gin.H{"items": []any{}})
		return
	}
	chs, err := s.notifiers.GetChannels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]gin.H, 0, len(chs))
	for _, ch := range chs {
		out = append(out, gin.H{
			"type": ch.Type, "webhook_url": ch.WebhookURL,
			"secret_set": ch.Secret != "", "enabled": ch.Enabled,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// PUT /api/admin/settings/notifications
func (s *Server) updateNotifiers(c *gin.Context) {
	if s.notifiers == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notifier unavailable"})
		return
	}
	var req []notifierChannelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// secret 留空表示不修改：按 type+webhook_url 匹配已有渠道，保留原 secret。
	existing, _ := s.notifiers.GetChannels(c.Request.Context())
	preserve := func(typ, url string) string {
		for _, ch := range existing {
			if ch.Type == typ && ch.WebhookURL == url {
				return ch.Secret
			}
		}
		return ""
	}
	chs := make([]notifier.Channel, 0, len(req))
	for _, r := range req {
		secret := r.Secret
		if secret == "" {
			secret = preserve(r.Type, r.WebhookURL)
		}
		chs = append(chs, notifier.Channel{
			Type: r.Type, WebhookURL: r.WebhookURL, Secret: secret, Enabled: r.Enabled,
		})
	}
	if err := s.notifiers.SaveChannels(c.Request.Context(), chs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GET /api/admin/settings/server
func (s *Server) getServerSettings(c *gin.Context) {
	cfg, err := s.store.GetServerSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"base_url": cfg.BaseURL})
}

// PUT /api/admin/settings/server
func (s *Server) updateServerSettings(c *gin.Context) {
	var req serverSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.BaseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "base_url 不能为空"})
		return
	}
	if err := s.store.UpdateBaseURL(c.Request.Context(), strings.TrimRight(req.BaseURL, "/")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
