# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 常用命令

后端（Go 1.25+，纯 Go，免 CGO）：

```bash
go build ./...                          # 编译全部包
go vet ./...                            # 静态检查
go test ./...                           # 跑全部测试（当前仓库尚无 *_test.go）
go test ./internal/analyzer -run TestScore -v   # 跑单个包/单个测试
go run ./cmd/server                     # 本地启动（见下方环境变量）
```

前端（`web/`，Vite + React + TS）：

```bash
cd web
npm ci
npm run dev      # Vite dev server :5173，代理 /api /hooks /public 到 :8080
npm run build    # tsc -b 类型检查 + vite build，产物输出到 web/dist
```

容器：

```bash
docker compose up -d --build            # 一条命令起服务（SQLite 默认）
docker build .                          # 多阶段：node 构建前端 → go build → alpine 运行
```

注意：后端用 `//go:embed all:web/dist` 嵌入前端（`embed.go`），`go build` 时 `web/dist` 必须存在。仓库里保留了构建产物/占位；若清空了 `web/dist`，先 `npm run build` 再编译 Go。

本地跑后端至少需要：

```bash
AICR_PI_AGENT_BIN=/path/to/pi-agent \
AICR_ADMIN_PASSWORD=dev123456 \
AICR_BASE_URL=http://localhost:8080 \
go run ./cmd/server
```

worker 实际调用 `pi-agent` 子进程完成审查；没有该二进制时服务能启动、入队，但任务会执行失败。完整环境变量见 `.env.example` 与 README「配置」。

## 功能代码目录（必读约定）

**每个功能都维护一份「代码位置目录」，统一记录在 [README.md 的「功能代码目录」章节](README.md#功能代码目录)。**

- 后端功能以 `internal/<package>/` 为单位；前端功能以 `web/src/pages/<feature>/`（必要时含 `api/<feature>.ts`、`components/`）为单位。
- 新增功能先确定归属包/目录，再在 README 目录表格追加一行（功能 → 主要代码位置）。
- 移动/拆分文件、新增 handler/store/migration/前端页面后，**必须同步更新**该目录。
- 修改某功能前先查该目录定位，不要凭猜测翻文件。

## 宏观架构

### 引导态 → 运行态的进程内热切换（关键、不直观）

「用哪个数据库」这件事本身不能存在数据库里，所以引入了引导文件 `$AICR_DATA_DIR/aicr.json`（`internal/bootstrap`）。整个生命周期在 `cmd/server/app.go` 的 `application` 里：

- 启动时若 `aicr.json` 不存在且 `AICR_DB_DRIVER` 等环境变量齐备 → 无头引导，直接写文件进入正式态；否则只装配「引导态」HTTP handler（仅 `/api/setup/*` 与 `/setup` 页），**不启动 worker**。
- 浏览器向导完成后回调 `completeBootstrap`：探测/建库（`store/probe.go`、`store/bootstrap.go`）、跑迁移（`store/migrate.go`）、写 `aicr.json`，然后 `buildRuntime` 装配全套依赖并通过 `setRuntime` 用 `sync.RWMutex` 原子替换 `application.handler` 和 `runtime`，再 `startWorker`。**全程不重启进程。**
- `application` 自己实现 `http.Handler`，每个请求读当前 `a.handler` 分发——这就是热切换的落点。改启动/路由逻辑时记住：存在两套 handler，引导态由 `internal/server/setup.go` 提供，正式态由 `server.Router()` 提供。

### 审查任务流水线

```
Git 平台 → POST /hooks/:token (webhook/handler.go)
        → parser 解析事件 + 签名校验 (webhook/{github,gitlab,gitee,coding}.go)
        → Enqueuer.EnqueueReview → jobs 表 (queue.Scheduler)
        → worker ClaimJob(租约) → 执行中 heartbeat 续约 → 失败指数退避重试
        → analyzer.Pipeline.HandleJob (analyzer/pipeline.go)
        → PiAgent CLI 子进程 (analyzer/piagent.go)：写 input.json、执行、读 report.json
        → scorer.Score 四维加权 + 严重度扣分 (analyzer/scorer.go)
        → 落库 reviews/findings (store/*_store.go)
        → notifier.Dispatcher 推送卡片 (notifier/{wecom,feishu,dingtalk}.go)
```

- 队列完全基于数据库表，无 Redis：`ClaimJob` 用 owner UUID + 租约抢占，`heartbeat` 续租，崩溃任务租约到期后可被重新认领，`idempotency_key` 去重。
- `PiAgent` 是接口（`pipeline.go`），`CLI` 是子进程实现，便于替换/测试。SSH 私钥**不写进 input.json**（`json:"-"`），而是落盘到 `$DATA_DIR/keys/repo-<id>`（0600）后通过 `GIT_SSH_COMMAND` 注入给 git；HTTPS token 才进 clone URL / input。
- 评分规则集中在 `analyzer/scorer.go`（架构 0.2 / 质量 0.3 / 安全 0.3 / 可维护 0.2，critical 封顶 70）；Pi Agent 只产出四维原始分与 findings。

### 依赖装配与接口隔离

`buildRuntime`（`cmd/server/app.go`）是唯一的手工依赖注入点：组装 `store` → `queue.Scheduler` → `analyzer.Pipeline`/`CLI` → `notifier.Dispatcher` → `webhook.Handler` → `adminService` → `server.Server`。`server.Server` 通过小接口（`ReviewStarter`、`JobAdmin`、`DashboardProvider`、`StatsProvider`、`NotifierSettings`）依赖上层，`adminService` 一个结构体实现多个接口——新增管理端能力时通常是给 `adminService` 加方法并在 `server.go` 接线，而不是改 `Server` 的具体依赖。

### 三种数据库方言

`store.Store` 持有 `Driver`（sqlite/postgres/mysql），所有查询经 `s.rebind()` 把 `?` 转成 postgres 的 `$1,$2…`（sqlite/mysql 保持 `?`），时间戳用 `s.now()`（pg `NOW()`、mysql `CURRENT_TIMESTAMP`、sqlite `datetime('now')`）。迁移是**三份且需手工同步**的：`internal/store/migrations/{sqlite,postgres,mysql}/*.sql`，版本号必须对齐（PG 与 MySQL 没有 0002，作者统计列合并进了 0001）。改 schema 时三个目录都要加迁移文件。

MySQL 方言的特殊点（改 SQL 时务必注意）：
- 驱动 `github.com/go-sql-driver/mysql`（纯 Go，`CGO_ENABLED=0` 仍成立），`openMySQL` 强制 `parseTime=true`（DATETIME 扫描为 `time.Time`）、`multiStatements=true`（整份迁移文件一次 Exec）、`charset=utf8mb4`；用户 DSN 里不写也会被补齐。
- 不支持 `UPDATE...RETURNING`：`ClaimJob` 对 MySQL 走事务 + `SELECT ... FOR UPDATE SKIP LOCKED` + UPDATE + SELECT（见 `job_store.go` 的 `claimJobMySQL`）；sqlite/postgres 仍用单条 `UPDATE...RETURNING`。
- upsert 用 `ON DUPLICATE KEY UPDATE`（settings），注意 `VALUES()` 引用；sqlite/postgres 用 `ON CONFLICT ... DO UPDATE`。
- MySQL 的 `TEXT`/`BLOB` 列不能有 `DEFAULT`；新增的无默认 TEXT 列要求相关 INSERT 显式传值（如 `CreateReview` 写 `summary/stats/error/score_dimensions=''`）。
- 标识列用 `VARCHAR(191)` 以兼容 utf8mb4 索引长度；`key` 是保留字，settings 的列名在 MySQL 下要加反引号。
- `ALTER TABLE ... ADD COLUMN` / 加外键在 MySQL 没有原生 `IF NOT EXISTS`，迁移里用 information_schema + 预处理语句（`PREPARE/EXECUTE`）保证幂等。
- 唯一冲突报错是 `Duplicate entry ... for key ...`（不含 "unique"），统一用 `isDuplicateErr()` 判断，不要只匹配 "unique"。
- 目标库不存在时 `probe.go` 会用去库名的维护连接 `CREATE DATABASE ... utf8mb4`（错误 1049）。

`modernc.org/sqlite` 也是纯 Go 实现。

### 前端

React 18 + Vite + Ant Design 5 + TanStack Query + react-router。`web/src/components/BootstrapGate.tsx` 在前端二次拦截未初始化状态跳 `/setup`；`api/client.ts` 是统一 axios 实例（注入 JWT、401 处理）。构建产物被 Go 嵌入二进制，前后端同源部署。

## 项目约定

- 面向用户的文档/说明用中文；代码标识符、commit message 用约定的英文 `type: 描述`（沿用现有历史，描述可为中文）。
- 启动期配置走环境变量（`internal/config`）；运行期可在 UI 修改的业务配置（LLM profiles、通知渠道、base URL、密码）存 DB `settings` 表，不要混进环境变量。
- 密钥类字段（LLM API Key、通知 secret、仓库 token、凭据私钥 `Credential.Secret`）API 响应中绝不返回；UI 脱敏回显、留空表示不修改。
- 公开报告用 `crypto/rand` 生成的 `public_token` 寻址，不暴露自增 id。
