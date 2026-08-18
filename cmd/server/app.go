package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	aicr "github.com/ai-code-review/aicr"
	"github.com/ai-code-review/aicr/internal/analyzer"
	"github.com/ai-code-review/aicr/internal/auth"
	"github.com/ai-code-review/aicr/internal/bootstrap"
	"github.com/ai-code-review/aicr/internal/config"
	"github.com/ai-code-review/aicr/internal/notifier"
	"github.com/ai-code-review/aicr/internal/queue"
	"github.com/ai-code-review/aicr/internal/server"
	"github.com/ai-code-review/aicr/internal/store"
	"github.com/ai-code-review/aicr/internal/webhook"
	"go.uber.org/zap"
)

// application 持有运行期依赖，支持引导完成后进程内热切换 HTTP handler 并启动 worker。
type application struct {
	cfg *config.Config
	log *zap.Logger

	mu       sync.RWMutex
	handler  http.Handler
	runtime  *runtime
	webFS    fs.FS

	workerCancel context.CancelFunc
	workerDone   chan struct{}
}

type runtime struct {
	st     *store.Store
	sqlDB  *sql.DB
	sched  *queue.Scheduler
}

func newApp(cfg *config.Config, log *zap.Logger) (*application, error) {
	webFS, err := fs.Sub(aicr.WebFS, "web/dist")
	if err != nil {
		return nil, err
	}
	return &application{cfg: cfg, log: log, webFS: webFS}, nil
}

func (a *application) Run() error {
	if err := os.MkdirAll(a.cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// 无头引导：环境变量指定了数据库且无引导文件，仅探测（PG 自动建库）并写引导文件，
	// 之后与「已初始化」路径统一 buildRuntime。
	if !bootstrap.Exists(a.cfg.DataDir) && a.cfg.DBDriver != "" {
		dsn := a.cfg.DBDSN
		if a.cfg.DBDriver == "sqlite" && dsn == "" {
			dsn = filepath.Join(a.cfg.DataDir, "aicr.db")
		}
		a.log.Info("headless bootstrap from env", zap.String("driver", a.cfg.DBDriver))
		if err := store.Ping(context.Background(), store.Driver(a.cfg.DBDriver), dsn); err != nil {
			return fmt.Errorf("headless bootstrap ping: %w", err)
		}
		if err := bootstrap.Save(a.cfg.DataDir, bootstrap.Config{Driver: a.cfg.DBDriver, DSN: dsn}); err != nil {
			return fmt.Errorf("headless bootstrap save: %w", err)
		}
	}

	httpSrv := &http.Server{
		Addr:              ":" + a.cfg.Port,
		Handler:           a,
		ReadHeaderTimeout: 10 * time.Second,
	}

	if bootstrap.Exists(a.cfg.DataDir) {
		bc, err := bootstrap.Load(a.cfg.DataDir)
		if err != nil {
			return err
		}
		rt, router, err := a.buildRuntime(bc.Driver, bc.DSN, a.cfg.AdminPassword, a.cfg.BaseURL)
		if err != nil {
			return err
		}
		a.setRuntime(rt, router)
		a.startWorker(rt)
		a.log.Info("database ready", zap.String("driver", bc.Driver))
	} else {
		// 未初始化：进入 setup 模式。
		setup := server.NewSetupHandler(a.log, a.webFS, a.cfg.DataDir, a.completeBootstrap)
		ginRouter := setup.RegisteredRouter()
		a.mu.Lock()
		a.handler = ginRouter
		a.mu.Unlock()
		a.log.Info("未检测到初始化配置，已进入 Web 引导向导", zap.String("addr", httpSrv.Addr))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		a.log.Info("http server listening", zap.String("addr", httpSrv.Addr))
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.log.Fatal("http server error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	a.log.Info("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)

	a.mu.RLock()
	rt := a.runtime
	a.mu.RUnlock()
	if a.workerCancel != nil {
		a.workerCancel()
	}
	if a.workerDone != nil {
		<-a.workerDone
	}
	if rt != nil {
		_ = rt.sqlDB.Close()
	}
	return nil
}

// ServeHTTP 让 application 成为可变 handler。
func (a *application) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	h := a.handler
	a.mu.RUnlock()
	h.ServeHTTP(w, r)
}

func (a *application) setRuntime(rt *runtime, h http.Handler) {
	a.mu.Lock()
	a.runtime = rt
	a.handler = h
	a.mu.Unlock()
}

// completeBootstrap 是 setup 向导的完成回调：开库迁移、写引导文件、装配运行时并热切换。
func (a *application) completeBootstrap(ctx context.Context, driver, dsn, adminPassword, baseURL string) error {
	drv := store.Driver(driver)
	if drv != store.DriverSQLite && drv != store.DriverPostgres && drv != store.DriverMySQL {
		return fmt.Errorf("不支持的数据库类型 %q", driver)
	}
	rt, router, err := a.buildRuntime(driver, dsn, adminPassword, baseURL)
	if err != nil {
		return err
	}
	if err := bootstrap.Save(a.cfg.DataDir, bootstrap.Config{Driver: driver, DSN: dsn}); err != nil {
		_ = rt.sqlDB.Close()
		return fmt.Errorf("写入引导文件: %w", err)
	}
	alreadyRunning := a.runtime != nil
	a.setRuntime(rt, router)
	if !alreadyRunning {
		a.startWorker(rt)
	}
	a.log.Info("初始化完成", zap.String("driver", driver))
	return nil
}

// buildRuntime 打开数据库、初始化设置并装配正式 HTTP 路由（不启动 worker）。
func (a *application) buildRuntime(driver, dsn, adminPassword, baseURL string) (*runtime, http.Handler, error) {
	drv := store.Driver(driver)
	db, err := store.Open(drv, dsn, a.log.Sugar())
	if err != nil {
		return nil, nil, err
	}
	st := store.New(db, drv)

	serverCfg, err := st.EnsureServerSettings(context.Background(), adminPassword, baseURL, a.cfg.JWTSecret)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("init server settings: %w", err)
	}
	jwtMgr := auth.NewManager(serverCfg.JWTSecret, 24*time.Hour)

	analyzer.CleanupOldWorkDirs(a.cfg.DataDir, 30*24*time.Hour, a.log)

	sched := queue.New(st, a.log, a.cfg.WorkerConcurrency)
	piAgent := analyzer.NewCLI(a.cfg.PiAgentBin, a.cfg.DataDir, a.log)
	pipeline := analyzer.NewPipeline(st, piAgent, a.log, a.cfg.DataDir)

	dispatcher := notifier.NewDispatcher(st, a.log, serverCfg.BaseURL)
	pipeline.SetNotifier(dispatcher)
	sched.Register("review", pipeline.HandleJob)

	enq := &reviewEnqueuer{q: sched}
	wh := webhook.NewHandler(st, a.log, enq)
	adminSvc := newAdminService(st, enq)

	srv := server.New(st, a.log, a.webFS, serverCfg.BaseURL, jwtMgr, wh, dispatcher, adminSvc, adminSvc, adminSvc, adminSvc)
	router := srv.Router()

	return &runtime{st: st, sqlDB: db, sched: sched}, router, nil
}

// startWorker 启动队列调度器（及配套的僵尸审查 reaper），返回可等待其退出的 channel。
func (a *application) startWorker(rt *runtime) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	a.workerCancel = cancel
	a.workerDone = done
	go func() {
		rt.sched.Start(ctx)
		close(done)
	}()
	a.startReaper(ctx, rt.st)
}
