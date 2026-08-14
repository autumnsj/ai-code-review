package server

// 登录
type loginReq struct {
	Password string `json:"password" binding:"required"`
}
type loginResp struct {
	Token string `json:"token"`
}

// 修改密码
type changePasswordReq struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// LLM 多模型配置（apiKey 逐条回显脱敏）
type llmProfileResp struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	BaseURL       string  `json:"base_url"`
	APIKeySet     bool    `json:"api_key_set"`
	APIKeyMasked  string  `json:"api_key_masked"`
	Model         string  `json:"model"`
	Temperature   float64 `json:"temperature"`
	MaxTokens     int     `json:"max_tokens"`
	TimeoutSec    int     `json:"timeout_sec"`
	ContextWindow int     `json:"context_window"`
	Enabled       bool    `json:"enabled"`
}

type llmSettingsResp struct {
	Profiles  []llmProfileResp `json:"profiles"`
	DefaultID string           `json:"default_id"`
}

type llmProfileReq struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	BaseURL       string  `json:"base_url"`
	APIKey        string  `json:"api_key"` // 留空表示不修改该 profile 的 key
	Model         string  `json:"model"`
	Temperature   float64 `json:"temperature"`
	MaxTokens     int     `json:"max_tokens"`
	TimeoutSec    int     `json:"timeout_sec"`
	ContextWindow int     `json:"context_window"`
	Enabled       bool    `json:"enabled"`
}

type llmSettingsReq struct {
	Profiles  []llmProfileReq `json:"profiles"`
	DefaultID string          `json:"default_id"`
}

type serverSettingsReq struct {
	BaseURL string `json:"base_url"`
}

type notifierChannelReq struct {
	Type       string `json:"type"`
	WebhookURL string `json:"webhook_url"`
	Secret     string `json:"secret"`
	Enabled    bool   `json:"enabled"`
}
