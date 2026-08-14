package server

import (
	"context"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ai-code-review/aicr/internal/server/middleware"
	"github.com/ai-code-review/aicr/internal/store"
)

// SetupCompleteFunc 由引导完成回调，负责实际开库、迁移、写引导文件并装配运行时。
type SetupCompleteFunc func(ctx context.Context, driver, dsn, adminPassword, baseURL string) error

// SetupHandler 首次启动时的初始化向导 HTTP（仅在未初始化时由 Router 挂载）。
type SetupHandler struct {
	log      *zap.Logger
	webFS    fs.FS
	dataDir  string
	complete SetupCompleteFunc
}

func NewSetupHandler(log *zap.Logger, webFS fs.FS, dataDir string, complete SetupCompleteFunc) *SetupHandler {
	return &SetupHandler{log: log, webFS: webFS, dataDir: dataDir, complete: complete}
}

type setupReq struct {
	Driver        string `json:"driver" binding:"required"`
	DSN           string `json:"dsn"`
	AdminPassword string `json:"admin_password" binding:"required,min=6"`
	BaseURL       string `json:"base_url" binding:"required"`
}

// RegisteredRouter 构建一个仅供引导期使用的 gin engine，注册 /api/setup/* 并回退到前端 SPA。
func (h *SetupHandler) RegisteredRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.SecurityHeaders())
	r.GET("/api/setup/status", h.status)
	r.POST("/api/setup/test", h.testConn)
	r.POST("/api/setup/complete", h.completeHandler)
	r.NoRoute(h.serveSetupSPA)
	return r
}

func (h *SetupHandler) status(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"initialized":   false,
		"sqlite_path":   h.dataDir + "/aicr.db",
		"data_dir":      h.dataDir,
	})
}

func (h *SetupHandler) testConn(c *gin.Context) {
	var req struct {
		Driver string `json:"driver" binding:"required"`
		DSN    string `json:"dsn"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var drv store.Driver
	switch req.Driver {
	case "sqlite":
		drv = store.DriverSQLite
	case "postgres":
		drv = store.DriverPostgres
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "driver 仅支持 sqlite 或 postgres"})
		return
	}
	dsn := req.DSN
	if drv == store.DriverSQLite && dsn == "" {
		dsn = h.dataDir + "/aicr.db"
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := store.Ping(ctx, drv, dsn); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *SetupHandler) completeHandler(c *gin.Context) {
	var req setupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Driver != "sqlite" && req.Driver != "postgres" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "driver 仅支持 sqlite 或 postgres"})
		return
	}
	dsn := req.DSN
	if req.Driver == "sqlite" && dsn == "" {
		dsn = h.dataDir + "/aicr.db"
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if err := h.complete(ctx, req.Driver, dsn, req.AdminPassword, req.BaseURL); err != nil {
		h.log.Warn("setup complete failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *SetupHandler) serveSetupSPA(c *gin.Context) {
	path := c.Request.URL.Path
	if strings.HasPrefix(path, "/api/") {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "service not initialized", "setup_required": true})
		return
	}
	if path != "/" {
		name := strings.TrimPrefix(path, "/")
		if f, err := h.webFS.Open(name); err == nil {
			f.Close()
			c.FileFromFS(name, http.FS(h.webFS))
			return
		}
	}
	data, err := fs.ReadFile(h.webFS, "index.html")
	if err != nil {
		c.String(http.StatusServiceUnavailable, "frontend not built")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}
