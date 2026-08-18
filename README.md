# AI Code Review

轻量级、可自托管的 AI 驱动代码审查与质量评估平台。Git 平台通过 Webhook 触发，自动调用 [Pi Agent](https://github.com/pi-ai/pi-agent) 完成代码拉取、静态检查与 LLM 语义分析，产出多维度评分报告，推送到企业微信 / 飞书 / 钉钉，并生成免登录公开报告页。

- **后端**：Go + Gin + zap，单二进制内嵌前端，支持 SQLite / PostgreSQL / MySQL（MariaDB），无 Redis 依赖
- **前端**：React + Vite + TypeScript + Ant Design
- **AI 基座**：Pi Agent（以子进程方式调用，封装全部审查逻辑）
- **一条命令启动**：`docker compose up`

## 功能

- 🔗 **多平台 Webhook**：GitHub / GitLab / Gitee / Coding，支持 Push 与 PR/MR 事件
- 🤖 **AI 审查引擎**：以 Pi Agent CLI 为基座，静态 + 语义分析，输出架构 / 质量 / 安全 / 可维护性四维度评分
- 📊 **质量报告**：评分环、维度明细、问题列表（严重度 / 文件 / 片段 / 建议）、公开免登录报告页
- 🔔 **通知推送**：企业微信 / 飞书 / 钉钉，支持加签，含报告直达链接
- 🛠️ **管理后台**：仓库管理、审查记录、任务队列（失败重试）、手动触发审查、概览仪表盘
- 🏆 **作者维度看板**：按作者聚合代码量（+/-）、四维均分、问题严重度分布，支持时间/仓库筛选与排行
- 🔁 **可靠异步**：数据库任务队列，租约 + 心跳 + 指数退避重试 + 幂等
- 🔒 **安全**：webhook 签名校验、仓库来源校验、不可枚举的公开 token、非 root 容器、禁执行仓库 git hooks

## 快速开始

```bash
cp .env.example .env   # 默认无需改动：首次访问走 Web 引导向导
docker compose up -d
```

访问 `http://localhost:8080`，首次启动会进入**初始化向导**：选择数据库（SQLite / PostgreSQL / MySQL）、测试连接、设置管理员密码与对外基础地址。向导完成后进程内热切换到正式服务（无需重启），用设置的密码登录（默认账号 `admin`；向导里不修改则为 `admin123`，登录后请在「设置 → 安全」修改）。

> 管理员密码与数据库选型仅在数据目录首次初始化时写入，之后修改环境变量不会改变已有配置。

> 容器内需要能调用到 Pi Agent。可将宿主机已安装的 `pi-agent` 二进制挂载进容器，
> 或基于本镜像构建一个内含 pi-agent 的镜像，并通过 `AICR_PI_AGENT_BIN` 指定路径。

### 本地开发

```bash
# 前端
cd web && npm install && npm run dev   # Vite dev server :5173，代理 /api /hooks /public 到 :8080

# 后端（另一个终端）
AICR_PI_AGENT_BIN=/path/to/pi-agent \
AICR_ADMIN_PASSWORD=dev123456 \
AICR_BASE_URL=http://localhost:8080 \
go run ./cmd/server
```

前端构建产物会被 `//go:embed all:web/dist` 嵌入二进制；本地直接 `go run` 前需先 `npm run build` 一次（或在 `web/dist` 保留占位）。

## 首次初始化向导

全新数据目录启动时，HTTP 服务只暴露初始化接口与引导页，不启动 worker：

1. 浏览器访问服务地址，自动跳转到 `/setup`。
2. 选择数据库：
   - **SQLite**：零配置，数据库文件默认放在 `$AICR_DATA_DIR/aicr.db`。
   - **PostgreSQL**：填写 host / port / 库名 / 用户 / 密码 / sslmode，或直接粘贴 DSN，例如
     `postgres://aicr:password@postgres:5432/aicr?sslmode=disable`。
     若目标库不存在，向导会先连到维护库 `postgres` 自动 `CREATE DATABASE`（库名仅允许字母/数字/`_`/`-`）。
   - **MySQL / MariaDB**：填写 host / port / 库名 / 用户 / 密码，或粘贴 DSN，例如
     `aicr:password@tcp(mysql:3306)/aicr?parseTime=true&loc=Local`。DSN 中的 `parseTime`/`multiStatements`/`charset=utf8mb4` 后端会自动补齐；目标库不存在时会以 `utf8mb4` 自动 `CREATE DATABASE`。
3. 「测试连接」验证可达性（PostgreSQL / MySQL 会顺带建库），通过后设置管理员密码与对外基础地址，「完成初始化」。
4. 完成后自动建表、启动 worker 并热切换到正式路由，随即进入登录页。

数据库选型与 DSN 记录在引导文件 `$AICR_DATA_DIR/aicr.json`（权限 `0600`）。它不能存进数据库本身（鸡生蛋问题），**迁移/备份时务必与数据库一并保管**——删除它会让服务重新进入向导。

### 无头（无向导）部署

自动化部署中没有浏览器可走向导时，用环境变量在首次启动时直接完成引导：

| 变量 | 说明 |
| --- | --- |
| `AICR_DB_DRIVER` | `sqlite`、`postgres` 或 `mysql`（留空则进入 Web 向导） |
| `AICR_DB_DSN` | PostgreSQL / MySQL 连接串；SQLite 留空即用默认 `aicr.db` |
| `AICR_ADMIN_PASSWORD` | 初始管理员密码 |
| `AICR_BASE_URL` | 对外基础地址 |

四个变量齐备且无 `aicr.json` 时，启动即自动写引导文件并进入正式模式，效果等价于走完向导。已有 `aicr.json` 时这些变量被忽略（重启幂等，不重置密码/数据库）。

使用外部数据库时，取消 `docker-compose.yml` 中注释的 `postgres` / `mysql` service 与 `depends_on`，并设置上面两个 `AICR_DB_*` 变量即可。

## 使用流程

1. 登录后台，在 **设置 → AI 模型** 配置一个或多个 OpenAI 兼容模型（base URL / API Key / 模型），并把其中一个设为**默认模型**（审查时使用）。
2. 在 **仓库** 页添加仓库，复制生成的 Webhook URL，配置到对应 Git 平台；如需签名校验，设置 Webhook Secret。
3. 推送代码或提 PR/MR，平台回调 webhook，自动创建审查任务并入队。
4. 在 **审查记录** 查看结果（运行中自动轮询），或在 **任务队列** 查看 / 重试失败任务。
5. 审查完成后：报告页可通过公开链接 `/reports/:token` 免登录访问；配置的通知群收到卡片。
6. 也可在仓库详情页 **手动触发审查**：指定 commit、指定分支（解析其 HEAD），或评审整个仓库（默认分支当前状态）。

## 配置

### 环境变量（部署期）

见 `.env.example`。所有 `AICR_*` 变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `AICR_PORT` | `8080` | 监听端口 |
| `AICR_DATA_DIR` | `/data` | 数据目录（引导文件、SQLite、工作目录） |
| `AICR_DB_DRIVER` | _(空)_ | `sqlite`/`postgres`/`mysql`；设置后启用无头首次启动，留空走 Web 向导 |
| `AICR_DB_DSN` | _(空)_ | PostgreSQL 或 MySQL 连接串；SQLite 留空即用 `$AICR_DATA_DIR/aicr.db` |
| `AICR_ADMIN_PASSWORD` | `admin123` | 首次启动初始化的管理员密码（仅全新数据目录生效） |
| `AICR_BASE_URL` | `http://localhost:8080` | 对外基础地址，用于通知中的报告链接 |
| `AICR_JWT_SECRET` | 随机 | JWT 签名密钥，留空启动时随机生成（重启会失效登录态） |
| `AICR_WORKER_CONCURRENCY` | `2` | worker 并发数（SQLite 写串行，建议 1–2；PostgreSQL / MySQL 可适当调高） |
| `AICR_LOG_LEVEL` | `info` | 日志级别 |
| `AICR_PI_AGENT_BIN` | `pi-agent` | Pi Agent 可执行命令 |

### UI 设置（运行期，存 SQLite）

- **AI 模型**：多个 OpenAI 兼容模型 profile（在 UI 中配置，不使用环境变量），选择一个为默认；API Key 脱敏回显、留空不修改
- **通知渠道**：多个 wecom / feishu / dingtalk webhook，secret 留空不修改
- **服务**：对外基础地址
- **安全**：修改管理员密码

## Pi Agent CLI 契约

本服务不实现自己的静态检查器或 LLM 客户端，而是把审查任务交给 Pi Agent。约定调用方式：

```
<AICR_PI_AGENT_BIN> run --input <workdir>/input.json --output <workdir>/report.json --workdir <workdir>
```

**输入 `input.json`**（由本服务生成）：

```json
{
  "repo_id": 12,
  "clone_url": "https://github.com/org/repo.git",
  "access_token": "",
  "commit_sha": "<sha>",
  "base_sha": "<sha>",
  "target_ref": "main",
  "source_ref": "feature/x",
  "pr": { "number": 7, "title": "...", "url": "...", "author": "alice" },
  "llm": { "base_url": "...", "api_key": "...", "model": "...", "temperature": 0.2, "max_tokens": 4096 }
}
```

**输出 `report.json`**（Pi Agent 生成，本服务解析落库）：

```json
{
  "summary": "整体质量良好，但存在一处高危...",
  "dimensions": {
    "architecture":  { "score": 85, "rationale": "..." },
    "quality":       { "score": 80, "rationale": "..." },
    "security":      { "score": 60, "rationale": "..." },
    "maintainability": { "score": 78, "rationale": "..." }
  },
  "findings": [
    {
      "rule_id": "sec-hardcoded",
      "severity": "high",
      "category": "security",
      "file_path": "app/config.py",
      "line": 10,
      "line_end": 10,
      "title": "硬编码密钥",
      "message": "发现疑似密钥硬编码",
      "snippet": "SECRET = 'abc123'",
      "suggestion": "改用环境变量",
      "confidence": "high",
      "source": "static"
    }
  ],
  "strengths": ["测试覆盖充分"],
  "risks": ["硬编码凭据"],
  "stats": { "files_changed": 3, "additions": 120, "deletions": 12 },
  "tokens_used": 5400,
  "truncated": false
}
```

字段约束：

- `score` 为 0–100 整数；`severity` 取值 `critical|high|medium|low|info`；`confidence` 取值 `high|medium|low`；`source` 取值 `static|llm`。
- `dimensions` 四维必填。总分由本服务按维度加权 + 严重度扣分计算（架构 0.2 / 质量 0.3 / 安全 0.3 / 可维护 0.2，critical 封顶 70）。
- Pi Agent 负责 clone / fetch、diff、静态检查、LLM 调用与重试，并在子进程超时（默认 10 分钟）内返回。

## 数据与备份

所有持久化数据位于 `AICR_DATA_DIR`（容器内 `/data`，compose 中为 named volume `aicr-data`）：

- `aicr.json`：引导文件，记录数据库选型与 DSN（权限 0600）
- `aicr.db`（使用 SQLite 时，WAL 模式）；使用 PostgreSQL / MySQL 时业务数据在外部库中
- `work/repo-<id>`：Pi Agent 按仓库持久化的工作目录（增量 fetch，不再每次 clone）；启动时保守清理 30 天未访问的残留

**SQLite 部署**：备份数据卷即可（建议先停服务或使用 SQLite `.backup` 以保证一致性）。
**PostgreSQL 部署**：业务数据用 `pg_dump` 备份，并**同时备份 `aicr.json`**（没有它服务不知道连哪个库）。
**MySQL / MariaDB 部署**：用 `mysqldump` 备份业务库，并同样备份 `aicr.json`。

## 发布（一键脚本）

改完代码后，在仓库根目录执行 `scripts/release.sh` 即可完成「构建前端 → 交叉编译 Go 二进制（已通过 `//go:embed` 内嵌前端）→ 上传到目标机 → 远端 `docker build` → `docker compose up -d` → 健康轮询」全流程。脚本不写死任何主机地址，所有环境相关值通过环境变量注入；也**不会**上传或覆盖目标机上的 `docker-compose.yml` / `aicr.json`（含数据卷与密钥，由目标机自行维护）。

```bash
DEPLOY_HOST=user@your-host \
DEPLOY_DIR=/opt/aicr/build-ctx \
COMPOSE_DIR=/opt/aicr \
./scripts/release.sh
```

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `DEPLOY_HOST` | （必填） | SSH 目标，`user@host` 或 SSH config 别名 |
| `DEPLOY_DIR` | `/opt/aicr/build-ctx` | 远端构建上下文目录（放 Dockerfile.release / 二进制 / deploy/） |
| `COMPOSE_DIR` | `/opt/aicr` | 远端 `docker-compose.yml` 所在目录 |
| `IMAGE` | `aicr:latest` | 构建出的镜像 tag |
| `CONTAINER` | `aicr` | 健康轮询的容器名 |
| `GOARCH` | `amd64` | 交叉编译目标架构（如 `arm64`） |
| `SKIP_WEB` | 空 | 非空则跳过前端构建（仅改后端时加速，复用现有 `web/dist`） |
| `SKIP_GO` | 空 | 非空则跳过 Go 构建（复用已编译好的二进制） |

前置条件：本地需 Go 1.25+、Node/npm；目标机需 Docker 与 docker compose，且 `docker-compose.yml`、`aicr.json` 已就位。脚本优先用 `rsync` 上传，两端均无 rsync 时回退到 `scp`。

## HTTP API（节选）

公开：

- `POST /hooks/:token` —— 各平台 webhook 入口
- `GET /public/healthz` —— 健康检查
- `GET /public/reviews/:token` —— 公开报告数据
- `GET /reports/:token` —— 公开报告页（前端）

管理端（`/api/admin/*`，需 `Authorization: Bearer <jwt>`）：

- `POST /login`、`POST /change-password`
- `GET|PUT /settings/llm`、`GET|PUT /settings/server`、`GET|PUT /settings/notifications`
- `GET|POST /repos`、`GET|PATCH|DELETE /repos/:id`、`POST /repos/:id/reset-token`、`POST /repos/:id/trigger`
- `GET /reviews`、`GET /reviews/:id`、`GET /reviews/:id/findings`
- `GET /jobs`、`POST /jobs/:id/retry`
- `GET /dashboard`
- `GET /stats/leaderboard?days=30&repo_id=&limit=10` —— 多指标排行榜（代码量 churn/新增/删除/审查次数/问题数 + 综合及四维均分），各取 Top N 一次返回
- `GET /stats/authors?days=30&repo_id=&sort=avg_score&page=1&page_size=50` —— 作者维度聚合（审查次数、四维均分、增删行合计、问题严重度计数）
- `GET /stats/authors/:author?days=30&repo_id=` —— 单作者明细（维度均分 + 最近审查）

## Webhook 签名

| 平台 | 匹配 Header | 签名方式 |
| --- | --- | --- |
| GitHub | `X-GitHub-Event` | `X-Hub-Signature-256`：HMAC-SHA256(raw body)，hex |
| GitLab | `X-Gitlab-Event` | `X-Gitlab-Token`：与 secret 常量时间比较 |
| Gitee | `X-Gitee-Event` | `X-Gitee-Token`：与 secret 常量时间比较 |
| Coding | `X-Coding-Event` | `X-Coding-Signature`：HMAC-SHA256(raw body)，`sha256=<hex>` |

未配置仓库 Webhook Secret 时跳过签名校验（仅靠 hook token 识别）；建议始终配置。

## 安全边界（请阅读）

- **密钥明文存储**：LLM API Key、通知 webhook secret、仓库 access token 在 MVP 中以明文存入数据库。PostgreSQL 的连接口令以 DSN 明文存放在 `$AICR_DATA_DIR/aicr.json`（权限 0600）。UI 脱敏回显、留空不修改，但请务必保护好数据卷与该文件的访问权限（后续可加 master key 加密）。
- **克隆不可信代码**：服务以非 root 用户运行，并通过 `core.hooksPath=/dev/null`、`GIT_TERMINAL_PROMPT=0` 禁止执行仓库内 git hooks，但仍建议部署在内网 / 受控环境，不要对公网开放任意仓库的 webhook。
- **公开报告**：`public_token` 使用 32 字节 `crypto/rand` 生成，不可枚举，不暴露自增 id；持有链接者可查看报告，请妥善分发。

## 功能代码目录

每个功能对应一组后端包 / 文件与前端页面 / API，改动某功能时按下表定位。新增功能后请同步维护此目录（项目 `CLAUDE.md` 约定）。

### 后端（Go，`internal/` 与 `cmd/server/`）

| 功能 | 主要代码位置 |
| --- | --- |
| 进程启动 / 装配 / 热切换 | `cmd/server/main.go`（入口）、`cmd/server/app.go`（`application` 生命周期、引导完成后进程内热切换 HTTP handler 与启动 worker/reaper）、`cmd/server/reaper.go`（定时回收卡在 pending/running 的僵尸审查：pending 超 10 分钟、running 超 35 分钟且无活跃 job 时标记 failed 并写日志）、`cmd/server/wire.go`（`reviewEnqueuer` 适配）、`cmd/server/service.go`（`adminService`：手动触发 / 任务管理 / dashboard 装配） |
| 启动期配置（环境变量） | `internal/config/config.go`（`AICR_*` 加载与校验） |
| 日志 | `internal/logger/logger.go`（zap 初始化） |
| 领域模型 | `internal/domain/domain.go`（Repo / Credential / Review / Finding / Job / LLMConfig 等纯模型，无外部依赖） |
| 首次初始化向导（后端） | `internal/bootstrap/bootstrap.go`（`$DATA_DIR/aicr.json` 读写）、`internal/store/bootstrap.go`（开库 / 建库 / 迁移）、`internal/store/probe.go`（连通性探测、PG 自动建库）、`internal/server/setup.go`（`/api/setup/*` 接口与完成回调） |
| HTTP 服务 / 路由 | `internal/server/server.go`（Gin 引擎、路由注册、依赖接口）、`internal/server/dto.go`（请求/响应结构体） |
| 管理端接口 | `internal/server/handler_admin.go`（登录、改密、LLM/服务/通知设置、模型列表拉取、`/settings/dimensions` 自定义打分维度；登录经 `middleware/loginguard.go` 按 IP 失败计数：5 次失败锁 10 分钟防暴力破解）、`internal/server/handler_repo.go`（仓库 CRUD、重置 token、`POST /repos/:id/webhook` 单个 / `POST /webhooks/register-all` 全部仓库一键注册 push webhook：已存在同 URL 的 hook 则删旧重建、自动补签名 secret、**注册前从平台拉取并回写真实默认分支**、逐个返回结果汇总，`ensureRepoWebhook` 复用）、`internal/server/handler_ops.go`（手动触发审查：commit/branch/repo 模式、任务重试）、`internal/server/handler_review.go`（审查记录 / findings / `GET /reviews/:id/log` 实时进度日志）、`internal/server/handler_stats.go`（作者维度聚合、`/stats/leaderboard` 多指标排行榜）、`internal/server/handler_credential.go`（可复用凭据库，含 provider/api_base_url）、`internal/server/handler_import.go`（平台仓库扫描批量**同步**：预览 / 提交为 upsert 语义——已存在的仓库不跳过，用平台最新值更新默认分支/网页地址/凭据/secret，并复用 `ensureRepoWebhook` 同步默认分支 + 删旧重建 push webhook；导出 `buildPlatformClientFromCreds` 供批量 webhook 注册复用）、`internal/server/handler_member.go`（成员备注 CRUD、未备注账号发现）、`internal/server/handler_public.go`（健康检查、公开报告数据） |
| 平台仓库 API 客户端 | `internal/platform/platform.go`（`Client` 接口 `Me/ListRepos/GetRepo/ResolveCommit/EnsureWebhook`、`New` 工厂、`sameHookURL`；`GetRepo` 用于 webhook 注册时同步真实默认分支）、`http.go`（GET/POST/DELETE/分页工具）、`github.go` / `gitlab.go` / `gitee.go` / `gitea.go`（四家平台实现，自建实例自定义 base URL；`EnsureWebhook` 先 list 按 URL 删除所有旧 hook（含历史重复项）再 create，用最新 secret 重建；Gitea 传顶层 `branch_filter`、GitLab 传 `push_events_branch_filter`，在**支持的版本**平台侧即只推默认分支（Gitea 1.18.x 会静默忽略该字段，属尽力而为）；GitHub/Gitee 无此能力。非默认分支的**权威过滤在服务端**（`webhook/handler.go` 收到非默认分支 push 立即返回 `ignored` 不入队）；`GetRepo` 拉取单仓库元数据同步默认分支；Gitea `ListRepos` 以 `/repos/search` 为主来源（返回 token 可见的全部仓库，含挂在他人个人账号下、自己作为协作者的仓库——这是 `/user/repos` 与 `/orgs/{org}/repos` 都会漏掉的场景），再合并 `/user/repos?affiliation=...` 与各组织仓库作为旧版本兼容兜底，全部按 full_name 去重） |
| 鉴权 / 限流 / 安全头中间件 | `internal/server/middleware/auth.go`（JWT）、`ratelimit.go`（IP 令牌桶）、`security.go`（安全响应头） |
| 认证（密码 / JWT） | `internal/auth/auth.go`（bcrypt、HS256 JWT 签发与校验） |
| Webhook 接入与签名 | `internal/webhook/handler.go`（`POST /hooks/:token` 入口、分发、push 仅审查默认分支、导出 `SameRepo`/`NormalizeCloneURL` 供导入去重复用）、`parser.go`（`Parser` 接口、`DefaultParsers`）、`github.go` / `gitlab.go` / `gitee.go` / `gitea.go` / `coding.go`（各平台事件解析与签名校验；Gitea 为新增） |
| AI 审查编排（Pi Agent） | `internal/analyzer/pipeline.go`（拉代码→调用→解析报告→落库全流程、读取自定义打分维度、`PiAgent` 接口、通过 ctx 注入 LogSink 把子进程输出逐行落库）、`piagent.go`（`CLI` 子进程调用、SSH key 注入 `GIT_SSH_COMMAND`、持久化工作目录 `work/repo-<id>` 增量 fetch、按 repoID 互斥 `sync.Map`、`bufio.Scanner` 并发捕获 stdout/stderr + 8KB ring buffer 保留错误尾部）、`logsink.go`（context 透传的逐行日志回调、`levelFor` 按内容分级）、`scorer.go`（按配置维度加权 + 严重度扣分计算综合分）、`cleanup.go`（持久 workdir 按 mtime 保守清理）；适配层 `deploy/pi-agent/index.mjs`（headless 驱动 Pi SDK，始终输出 `[stage]`/`[tool]` 进度行到 stderr，并把每次工具调用的真实输出以 `  │` 前缀逐行回显；启动时清理残留 `*.lock`、checkout 失败自动删库重克隆自愈；**不把 diff 内容塞进 prompt**，只给 base/head 提交与文件清单（+/- 行数），让 agent 用 bash/read/grep 逐文件 `git diff <base><head> -- <path>` 自查，避免大 diff 撑爆上下文；**审查范围/时长护栏（后台「设置 → 审查范围」可配，DB `settings.review_limits`，有默认值）**：① 提交时间窗口（默认 5 天）——以 head 为基准，触发方给的 base 跨越更久时，在 JS 内按提交时间过滤把 base 收窄到窗口内最早提交的父提交（根提交回退空树），不依赖 git `--since` 的 approxidate；② 文件数上限（默认 40）——窗口内文件过多时按「区间内最后修改时间」排序，只把最近改动的一批列入清单交给 AI（其余计入总数但不审）；③ 墙钟硬超时（默认 600s≈10 分钟）——`setTimeout` 到点 `session.abort()` 并注入收束提示强制提交。收窄/抽样/超时结果写入 `stats`（`range_*`/`window_days`/`max_files`/`files_limited`/`timed_out`），报告页与通知展示实际区间。护栏规则同时写进 `deploy/pi-agent/skills/code-review/SKILL.md`（AI 只审清单文件、到点交卷、看不完不编造并在 summary 注明抽样）；**文件/工具预算**：按抽样后文件数算出文件预算（5–15 个，对数增长）与工具调用预算，工具调用达硬上限（预算×2）则 `session.abort()` 强制收束提交，避免大仓库审查失控/OOM；**bash 噪声降级**：grep/find/git 等搜索命令退出码 1 仅表示无匹配，记为 `[info] 搜索无命中` 而非 `[warn] 执行失败`（真正的 no such file / permission denied / fatal 仍告警）；**codegraph 集成**：`prepareCodeGraph` 在 workdir 外建持久索引目录并以相对软链 `src/.codegraph` 指向它（`git clean -fdx` 只删软链、索引库留存增量 `sync`），首次 `codegraph init -f`、后续 `codegraph sync -q`，全部 `CODEGRAPH_NO_*` 非交互环境变量，失败优雅降级回 grep/read；prompt 引导 agent 优先用 `codegraph explore/node/callers` 精确定位符号与调用链；**多作者归属**：审查完成后对每条 finding 跑 `git blame --line-porcelain -L <line>,<line> <head> -- <file>` 解析行作者 email，同时 `git log --numstat <base>..<head>` 汇总 `participants`（去重提交者）与每作者 `author_stats`（增删行/改动文件）写入报告，供后端拆分个人报告；镜像通过 `Dockerfile.release` 全局安装 `@colbymchenry/codegraph`） |
| 异步任务队列 | `internal/queue/queue.go`（基于 `jobs` 表的调度器、worker pool、租约 / 心跳 / 幂等键；**审查任务不做失败重试**，一次失败即 dead，由用户手动重跑；worker 崩溃/重启导致租约过期的任务仍会被重新认领完成） |
| 通知推送 | `internal/notifier/dispatcher.go`（读取渠道、分发、组装卡片；`NotifyAuthorReview` 按作者逐条发送）、`notifier.go`（`Channel`、整体 `BuildMarkdown` 与个人 `BuildAuthorMarkdown`）、`wecom.go` / `feishu.go` / `dingtalk.go`（各平台 webhook 与加签） |
| 多作者拆分与归属 | `internal/analyzer/pipeline.go` 的 `persistAuthorReports`（AI 只审一次全量 base..head，按 pi-agent 的 `git blame` 归属把 findings 分桶给每位参与者，各自 `scorer.Score` 独立评分、写 `review_author_reports`、各自公开 token；blame 到区间外旧作者的 finding 只留在全量报告）、`author_report_store.go`（按 `(review_id,author)` 方言 upsert、按 token 查公开报告、`DeleteAuthorReports`）；迁移 `0006_author_reports.sql`（三方言，含为历史 succeeded 审查回填一条作者报告）；`internal/store/author_store.go` 看板聚合改读 `review_author_reports`；公开端点 `GET /public/author-reports/:token`、管理端点 `GET /api/admin/reviews/:id/author-reports` |
| SSH 密钥 | `internal/sshkey/sshkey.go`（ed25519 生成、私钥解析、公钥行与 SHA256 指纹） |
| 数据访问与迁移 | `internal/store/store.go`（`Store` 聚合、方言/占位符转换）、`db.go`（打开连接，`openMySQL` 强制 `parseTime`/`multiStatements`/`utf8mb4`）、`probe.go`（连通性探测、PG/MySQL 自动建库）、`migrate.go`（嵌入式 SQL 迁移执行）、`migrations/{sqlite,postgres,mysql}/*.sql`；按领域拆分：`repo_store.go`、`credential_store.go`、`review_store.go`、`review_log_store.go`（审查过程逐行进度日志，`AppendReviewLog`/`ListReviewLogs(sinceID)` 增量拉取）、`reaper_store.go`（`ListActiveJobReviewIDs`/`ListStaleReviewIDs` 供僵尸审查回收）、`review_store.go` 的 `MarkReviewFailed`（reaper 标记超时失败）、`finding_store.go`（含 `ListFindingsByAuthor`、`author` 列）、`author_report_store.go`（按 `(review_id,author)` 方言 upsert、公开 token 查询、删除）、`job_store.go`（MySQL 用 `FOR UPDATE SKIP LOCKED` 抢占任务）、`settings_store.go`（upsert 按方言分支）、`dashboard_store.go`、`author_store.go`（作者维度聚合，数据源改读 `review_author_reports`）、`author_alias_store.go`（成员备注表 CRUD、未备注 login 发现） |
| 前端资源嵌入 | `embed.go`（`//go:embed all:web/dist`） |

### 前端（React，`web/src/`）

| 功能 | 主要代码位置 |
| --- | --- |
| 入口 / 路由 / 引导门禁 | `main.tsx`、`router.tsx`、`components/BootstrapGate.tsx`（未初始化时拦截跳转 `/setup`）、`components/AdminLayout.tsx`（后台布局）、`components/Footer.tsx` |
| HTTP 客户端 | `api/client.ts`（axios 实例、JWT 注入、401 处理） |
| 首次初始化向导 | `pages/setup/index.tsx`、`api/setup.ts` |
| 登录 | `pages/login/index.tsx` |
| 仪表盘 | `pages/dashboard/index.tsx`、`api/ops.ts` |
| 仓库管理 / 批量导入 | `pages/repos/index.tsx`、`pages/repos/detail.tsx`（手动触发 commit/branch/repo）、`pages/repos/ImportWizard.tsx`（平台扫描三步导入向导）、`api/repos.ts`（`importPreview`/`importCommit`/`trigger`/`registerWebhook`/`registerAllWebhooks`；仓库页「一键注册全部 Webhook」按钮，逐个回显新增/已存在/跳过/失败） |
| 审查记录 / 报告 | `pages/reviews/index.tsx`、`pages/reviews/detail.tsx`（含 `ReviewLogPanel.tsx`：2s 增量轮询 `/reviews/:id/log`，终端样式实时展示 AI 进度，上滚暂停自动跟随；多作者审查展示「作者报告」表，各人评分/问题数/改动量与个人公开报告链接）、`pages/public/report.tsx`（整体免登录公开页）、`pages/public/authorReport.tsx`（按作者拆分的个人公开页 `/author-reports/:token`）、`api/reviews.ts`（`dimensionRows`、`AuthorReport`、`authorReports`/`publicAuthorGet`、`logs`/`ReviewLog`） |
| 任务队列 | `pages/jobs/index.tsx`、`api/ops.ts` |
| 作者维度看板 | `pages/stats/authors.tsx`（真名 / @login / 团队展示）、`pages/stats/leaderboard.tsx`（单页多榜柱形图：代码量/审查次数/问题数 + 各维度评分，CSS 横向柱、奖牌排名，支持时间/仓库筛选）、`api/stats.ts`（`leaderboard`） |
| 成员备注 | `pages/members/index.tsx`（CRUD + 未备注账号快捷添加）、`api/members.ts` |
| 可复用凭据库 | `pages/credentials/index.tsx`（含所属平台 / API 地址字段）、`api/credentials.ts` |
| 设置（AI 模型 / 通知 / 服务 / 安全 / 打分维度） | `pages/settings/index.tsx`（「打分维度」Tab，`Form.List` 自定义维度）、`api/settings.ts` |
| 通用组件 | `components/ScoreRing.tsx`（评分环）、`components/SeverityTag.tsx`（严重度标签） |
| 构建 / 代理 | `web/vite.config.ts`（dev 代理 `/api` `/hooks` `/public` 到后端）、`web/package.json` |

### 数据库迁移

- SQLite：`0001_init.sql`、`0002_author_stats.sql`、`0003_credentials.sql`、`0004_member_and_dimensions.sql`、`0005_review_logs.sql`、`0006_author_reports.sql`
- PostgreSQL：`0001_init.sql`、`0003_credentials.sql`、`0004_member_and_dimensions.sql`、`0005_review_logs.sql`、`0006_author_reports.sql`（作者统计列随 0001 一并建表；新增迁移需两边同步）
- MySQL / MariaDB：`0001_init.sql`、`0003_credentials.sql`、`0004_member_and_dimensions.sql`、`0005_review_logs.sql`、`0006_author_reports.sql`（作者统计列随 0001 一并建表；`ALTER ADD COLUMN`/外键用 information_schema + 预处理保证幂等）
- `0004_member_and_dimensions.sql`：`authors` 成员备注表、`credentials` 增 `provider`/`api_base_url`、`reviews` 增 `score_dimensions`（自定义维度分数 JSON）
- `0005_review_logs.sql`：`review_logs` 表（审查过程逐行进度日志，按 `review_id,id` 增量拉取）
- `0006_author_reports.sql`：`findings` 增 `author` 列（blame 归属 email）；`review_author_reports` 表（每参与者一条独立报告、各自评分/计数/公开 token），并为历史 succeeded 审查回填一条
- **新增迁移需三种方言同步**，版本号对齐；PG/MySQL 没有 0002（作者统计列合并进 0001）。MySQL 迁移文件靠 DSN 的 `multiStatements=true` 整体执行（后端自动开启）。

## 技术栈

- 后端：Gin、zap、`modernc.org/sqlite`（纯 Go，免 CGO）、`golang-jwt/jwt/v5`、`golang.org/x/crypto/bcrypt`、`google/uuid`
- 前端：React 18、Vite 5、TypeScript、Ant Design 5、React Router 6、TanStack Query v5、axios
- 部署：Docker 多阶段构建（node → golang → alpine）、tini、非 root 用户

## License

MIT
