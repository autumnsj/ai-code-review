# AI Code Review

轻量级、可自托管的 AI 驱动代码审查与质量评估平台。Git 平台通过 Webhook 触发，自动调用 [Pi Agent](https://github.com/pi-ai/pi-agent) 完成代码拉取、静态检查与 LLM 语义分析，产出多维度评分报告，推送到企业微信 / 飞书 / 钉钉，并生成免登录公开报告页。

- **后端**：Go + Gin + zap，单二进制内嵌前端，支持 SQLite / PostgreSQL，无 Redis 依赖
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

访问 `http://localhost:8080`，首次启动会进入**初始化向导**：选择数据库（SQLite / PostgreSQL）、测试连接、设置管理员密码与对外基础地址。向导完成后进程内热切换到正式服务（无需重启），用设置的密码登录（默认账号 `admin`；向导里不修改则为 `admin123`，登录后请在「设置 → 安全」修改）。

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
3. 「测试连接」验证可达性（PostgreSQL 会顺带建库），通过后设置管理员密码与对外基础地址，「完成初始化」。
4. 完成后自动建表、启动 worker 并热切换到正式路由，随即进入登录页。

数据库选型与 DSN 记录在引导文件 `$AICR_DATA_DIR/aicr.json`（权限 `0600`）。它不能存进数据库本身（鸡生蛋问题），**迁移/备份时务必与数据库一并保管**——删除它会让服务重新进入向导。

### 无头（无向导）部署

自动化部署中没有浏览器可走向导时，用环境变量在首次启动时直接完成引导：

| 变量 | 说明 |
| --- | --- |
| `AICR_DB_DRIVER` | `sqlite` 或 `postgres`（留空则进入 Web 向导） |
| `AICR_DB_DSN` | PostgreSQL 连接串；SQLite 留空即用默认 `aicr.db` |
| `AICR_ADMIN_PASSWORD` | 初始管理员密码 |
| `AICR_BASE_URL` | 对外基础地址 |

四个变量齐备且无 `aicr.json` 时，启动即自动写引导文件并进入正式模式，效果等价于走完向导。已有 `aicr.json` 时这些变量被忽略（重启幂等，不重置密码/数据库）。

使用 PostgreSQL 时取消 `docker-compose.yml` 中注释的 `postgres` service 与 `depends_on`，并设置上面两个 `AICR_DB_*` 变量即可。

## 使用流程

1. 登录后台，在 **设置 → AI 模型** 配置一个或多个 OpenAI 兼容模型（base URL / API Key / 模型），并把其中一个设为**默认模型**（审查时使用）。
2. 在 **仓库** 页添加仓库，复制生成的 Webhook URL，配置到对应 Git 平台；如需签名校验，设置 Webhook Secret。
3. 推送代码或提 PR/MR，平台回调 webhook，自动创建审查任务并入队。
4. 在 **审查记录** 查看结果（运行中自动轮询），或在 **任务队列** 查看 / 重试失败任务。
5. 审查完成后：报告页可通过公开链接 `/reports/:token` 免登录访问；配置的通知群收到卡片。
6. 也可在仓库详情页 **手动触发审查**，指定 commit SHA。

## 配置

### 环境变量（部署期）

见 `.env.example`。所有 `AICR_*` 变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `AICR_PORT` | `8080` | 监听端口 |
| `AICR_DATA_DIR` | `/data` | 数据目录（引导文件、SQLite、工作目录） |
| `AICR_DB_DRIVER` | _(空)_ | `sqlite`/`postgres`；设置后启用无头首次启动，留空走 Web 向导 |
| `AICR_DB_DSN` | _(空)_ | PG 连接串；SQLite 留空即用 `$AICR_DATA_DIR/aicr.db` |
| `AICR_ADMIN_PASSWORD` | `admin123` | 首次启动初始化的管理员密码（仅全新数据目录生效） |
| `AICR_BASE_URL` | `http://localhost:8080` | 对外基础地址，用于通知中的报告链接 |
| `AICR_JWT_SECRET` | 随机 | JWT 签名密钥，留空启动时随机生成（重启会失效登录态） |
| `AICR_WORKER_CONCURRENCY` | `2` | worker 并发数（SQLite 写串行，建议 1–2；PG 可适当调高） |
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
- `aicr.db`（使用 SQLite 时，WAL 模式）；使用 PostgreSQL 时业务数据在外部库中
- `work/`：Pi Agent 单次审查工作目录，审查结束自动清理，启动时清理超过 1 天的残留

**SQLite 部署**：备份数据卷即可（建议先停服务或使用 SQLite `.backup` 以保证一致性）。
**PostgreSQL 部署**：业务数据用 `pg_dump` 备份，并**同时备份 `aicr.json`**（没有它服务不知道连哪个库）。

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

## 技术栈

- 后端：Gin、zap、`modernc.org/sqlite`（纯 Go，免 CGO）、`golang-jwt/jwt/v5`、`golang.org/x/crypto/bcrypt`、`google/uuid`
- 前端：React 18、Vite 5、TypeScript、Ant Design 5、React Router 6、TanStack Query v5、axios
- 部署：Docker 多阶段构建（node → golang → alpine）、tini、非 root 用户

## License

MIT
