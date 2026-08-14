package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/sshkey"
	"github.com/ai-code-review/aicr/internal/store"
)

type credentialResp struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	PublicKey     string `json:"public_key,omitempty"`
	Fingerprint   string `json:"fingerprint,omitempty"`
	SecretSet     bool   `json:"secret_set"`
	SecretMasked  string `json:"secret_masked,omitempty"`
	CreatedAt     string `json:"created_at"`
}

type credentialWithKeyResp struct {
	credentialResp
	// PrivateKey 仅在创建自动生成的 SSH 密钥时一次性返回，之后不再提供。
	PrivateKey string `json:"private_key,omitempty"`
}

func (s *Server) toCredentialResp(c *domain.Credential) credentialResp {
	resp := credentialResp{
		ID:          c.ID,
		Name:        c.Name,
		Type:        c.Type,
		PublicKey:   c.PublicKey,
		Fingerprint: c.Fingerprint,
		CreatedAt:   c.CreatedAt.Format("2006-01-02 15:04"),
	}
	if c.Type == domain.CredentialHTTPSToken {
		resp.SecretSet = c.Secret != ""
		resp.SecretMasked = maskKey(c.Secret)
	} else {
		resp.SecretSet = c.Secret != ""
	}
	return resp
}

type createCredentialReq struct {
	Name   string `json:"name" binding:"required"`
	Type   string `json:"type" binding:"required"`
	Secret string `json:"secret"` // 粘贴的 SSH 私钥，或 HTTPS token
}

// POST /api/admin/credentials
func (s *Server) createCredential(c *gin.Context) {
	var req createCredentialReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称不能为空"})
		return
	}

	in := store.CreateCredentialInput{Name: req.Name}
	var oneTimePrivateKey string

	switch req.Type {
	case domain.CredentialSSH:
		if strings.TrimSpace(req.Secret) != "" {
			// 粘贴已有私钥：校验并派生公钥/指纹
			kp, err := sshkey.ParsePrivateKey([]byte(req.Secret))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			in.Secret = kp.PrivatePEM
			in.PublicKey = kp.Public
			in.Fingerprint = kp.Fingerprint
		} else {
			// 自动生成新密钥对
			kp, err := sshkey.GenerateEd25519()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "生成密钥失败: " + err.Error()})
				return
			}
			in.Secret = kp.PrivatePEM
			in.PublicKey = kp.Public
			in.Fingerprint = kp.Fingerprint
			oneTimePrivateKey = kp.PrivatePEM
		}
		in.Type = domain.CredentialSSH
	case domain.CredentialHTTPSToken:
		if strings.TrimSpace(req.Secret) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "HTTPS Token 不能为空"})
			return
		}
		in.Type = domain.CredentialHTTPSToken
		in.Secret = req.Secret
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "type 必须为 ssh 或 https_token"})
		return
	}

	cred, err := s.store.CreateCredential(c.Request.Context(), in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := credentialWithKeyResp{credentialResp: s.toCredentialResp(cred), PrivateKey: oneTimePrivateKey}
	c.JSON(http.StatusCreated, resp)
}

// GET /api/admin/credentials
func (s *Server) listCredentials(c *gin.Context) {
	creds, err := s.store.ListCredentials(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]credentialResp, 0, len(creds))
	for _, cred := range creds {
		out = append(out, s.toCredentialResp(cred))
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

type updateCredentialReq struct {
	Name   *string `json:"name"`
	Secret *string `json:"secret"`
}

// PATCH /api/admin/credentials/:id
func (s *Server) updateCredential(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req updateCredentialReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing, err := s.store.GetCredential(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if req.Secret != nil && strings.TrimSpace(*req.Secret) != "" {
		if existing.Type == domain.CredentialSSH {
			kp, err := sshkey.ParsePrivateKey([]byte(*req.Secret))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if err := s.store.UpdateCredentialKeyMaterial(c.Request.Context(), id, kp.PrivatePEM, kp.Public, kp.Fingerprint); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		} else if err := s.store.UpdateCredential(c.Request.Context(), id, nil, req.Secret); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if err := s.store.UpdateCredential(c.Request.Context(), id, &name, nil); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	cred, err := s.store.GetCredential(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s.toCredentialResp(cred))
}

// DELETE /api/admin/credentials/:id
func (s *Server) deleteCredential(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := s.store.DeleteCredential(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
