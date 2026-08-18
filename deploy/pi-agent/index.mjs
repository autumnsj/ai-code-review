#!/usr/bin/env node
// pi-agent —— AICR 审查平台与 Pi Coding Agent 之间的 headless 适配层。
//
// 平台（internal/analyzer/piagent.go）以子进程方式调用：
//   pi-agent run --input <input.json> --output <report.json> --workdir <dir>
// Pi 本身没有这个子命令，这里用 Pi SDK 在进程内完成一次非交互审查，
// 并把结果写成平台约定的 report.json（见 internal/analyzer/pipeline.go 的
// PiAgentInput / PiAgentReport）。Go 侧代码无需任何改动。
//
// 安全：agent 只被授予 read/grep/find/ls/bash（用于 git diff 等只读命令），
// 不授予 write/edit；不加载被审仓库内的 AGENTS.md，避免仓库内容反向操纵审查。

import { spawnSync } from "node:child_process";
import { mkdirSync, writeFileSync, readFileSync, existsSync, rmSync, readdirSync, unlinkSync, lstatSync, symlinkSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { setGlobalDispatcher, Agent } from "undici";
import {
  createAgentSession,
  DefaultResourceLoader,
  ModelRuntime,
  SessionManager,
  SettingsManager,
} from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";

// 给所有出站 HTTP（LLM API）设硬超时，防止单个请求挂起导致整个审查僵死
// （观察到某些大请求会一直 ESTABLISHED 但无数据，进程卡在 ep_poll）。
setGlobalDispatcher(new Agent({
  connect: { timeout: 15_000 },
  headersTimeout: 300_000,
  bodyTimeout: 300_000,
}));

const __dirname = dirname(fileURLToPath(import.meta.url));

// ---------- 参数解析 ----------
function parseArgs(argv) {
  const args = argv.slice(2);
  if (args[0] !== "run") {
    throw new Error(`不支持的子命令：${args[0] ?? "(空)"}，仅支持 "run"`);
  }
  const opts = {};
  for (let i = 1; i < args.length; i++) {
    if (args[i] === "--input") opts.input = args[++i];
    else if (args[i] === "--output") opts.output = args[++i];
    else if (args[i] === "--workdir") opts.workdir = args[++i];
  }
  if (!opts.input || !opts.output || !opts.workdir) {
    throw new Error("用法：pi-agent run --input <path> --output <path> --workdir <dir>");
  }
  return opts;
}

// ---------- git 封装 ----------
function git(cwd, args, extraEnv = {}) {
  const r = spawnSync("git", args, {
    cwd,
    encoding: "utf8",
    env: { ...process.env, GIT_TERMINAL_PROMPT: "0", ...extraEnv },
    maxBuffer: 256 * 1024 * 1024,
  });
  if (r.status !== 0) {
    throw new Error(`git ${args.join(" ")} 失败：${(r.stderr || r.stdout || "").trim()}`);
  }
  return (r.stdout || "").trim();
}

// 把可能含内联凭据的 https URL 归一化为带 access_token 的形式。
function authenticatedCloneURL(cloneURL, token) {
  if (!token || !/^https?:\/\//i.test(cloneURL)) return cloneURL;
  const url = new URL(cloneURL);
  url.username = encodeURIComponent(token);
  url.password = "";
  return url.toString();
}

// 清理上次审查被强杀（25 分钟超时 / 容器重启）时 git 残留的 *.lock 文件。
// Go 侧已按 repoID 串行（repoLocks），进入这里时该仓库不可能有其它活着的 git 进程，
// 因此 .git 下所有锁文件都是孤儿，可安全删除，否则 checkout/fetch 会报 index.lock exists。
function pruneStaleLocks(gitDir) {
  const removed = [];
  const walk = (dir) => {
    let entries;
    try { entries = readdirSync(dir, { withFileTypes: true }); } catch { return; }
    for (const e of entries) {
      const p = join(dir, e.name);
      if (e.isDirectory()) {
        walk(p);
      } else if (e.isFile() && e.name.endsWith(".lock")) {
        try {
          unlinkSync(p);
          removed.push(p);
        } catch { /* 忽略删除失败 */ }
      }
    }
  };
  walk(gitDir);
  if (removed.length > 0) {
    process.stderr.write(`[stage] 清理了 ${removed.length} 个残留 git 锁文件（上次进程可能被中断）\n`);
  }
}

// 准备被审仓库到 <workdir>/src：没有则 clone，有则 fetch 并 checkout 到目标 commit。
function prepareRepo(workdir, input) {
  const src = join(workdir, "src");
  mkdirSync(src, { recursive: true });
  const cloneURL = authenticatedCloneURL(input.clone_url, input.access_token);

  if (!existsSync(join(src, ".git"))) {
    // 部分克隆：只拉提交/树对象，blob 按需获取（checkout/diff 时才拉改动文件）。
    // 对大仓库可把首次拉取从数百 MB 降到几十 MB；服务端不支持 filter 时回退到浅克隆。
    try {
      git(workdir, [
        "clone", "--no-tags", "--no-checkout", "--quiet",
        "--filter=blob:none",
        cloneURL, src,
      ]);
    } catch {
      rmSync(src, { recursive: true, force: true });
      mkdirSync(src, { recursive: true });
      git(workdir, [
        "clone", "--no-tags", "--no-checkout", "--quiet",
        "--depth", "200",
        cloneURL, src,
      ]);
    }
  } else {
    // 先清理上次被强杀残留的 git 锁，否则 fetch/checkout 会因 index.lock exists 失败。
    pruneStaleLocks(join(src, ".git"));
    // 更新 remote（凭据可能轮换）并增量抓取。
    git(src, ["remote", "set-url", "origin", cloneURL]);
    git(src, ["fetch", "--no-tags", "--prune", "--quiet", "origin", "+refs/heads/*:refs/remotes/origin/*"]);
  }

  // 只读硬保证：把 origin 的推送地址指向无效值。fetch/clone 仍用上面的真实地址，
  // 但 agent 即使通过 bash 执行 git push（含 force push）也会立即失败，无法写回远端。
  git(src, ["remote", "set-url", "--push", "origin", "no-push://read-only-review"]);

  // 抓取目标 commit（浅克隆可能尚未包含）；按 sha 抓取在 Gitea/GitHub/GitLab 均支持。
  if (input.commit_sha) {
    try {
      git(src, ["fetch", "--no-tags", "--quiet", "--depth", "200", "origin", input.commit_sha]);
    } catch {
      // 某些服务端不允许按 sha fetch；若前面的 fetch 已拿到可忽略。
    }
    try {
      git(src, ["checkout", "--quiet", "--detach", input.commit_sha]);
      git(src, ["reset", "--hard", "--quiet", input.commit_sha]);
      git(src, ["clean", "-fdx", "--quiet"]);
    } catch (err) {
      // worktree 可能因上次中断/磁盘问题损坏（不止锁文件）。删除后重新浅克隆自愈。
      process.stderr.write(`[warn] checkout 到目标提交失败（${err.message.split("\n")[0]}），删除工作区后重新克隆…\n`);
      rmSync(src, { recursive: true, force: true });
      mkdirSync(src, { recursive: true });
      git(workdir, [
        "clone", "--no-tags", "--no-checkout", "--quiet",
        "--depth", "200", cloneURL, src,
      ]);
      git(src, ["remote", "set-url", "--push", "origin", "no-push://read-only-review"]);
      git(src, ["fetch", "--no-tags", "--quiet", "--depth", "200", "origin", input.commit_sha]);
      git(src, ["checkout", "--quiet", "--detach", input.commit_sha]);
      git(src, ["reset", "--hard", "--quiet", input.commit_sha]);
      git(src, ["clean", "-fdx", "--quiet"]);
    }
  }
  return src;
}

// 在被审仓库准备 codegraph 索引，供 agent 用 `codegraph explore/node` 精确定位符号与调用路径，
// 替代大量盲目的 grep/read，减少上下文检索量与工具调用次数、缩短审查时间。
//
// 索引数据放在 <workdir>/codegraph（随持久 workdir 跨审查复用，增量 sync），src/.codegraph
// 是指向它的相对软链。这样 prepareRepo 里的 `git clean -fdx`（会删未跟踪文件）最多删掉软链本身，
// 不会删掉外链的索引库；本函数每次重建软链，首次 init、之后 sync。
//
// 索引失败不阻断审查（降级回 grep/read），故全程 try/catch 并给 spawnSync 设超时。
function prepareCodeGraph(workdir, src) {
  const dataDir = join(workdir, "codegraph");
  try {
    mkdirSync(dataDir, { recursive: true });
    const link = join(src, ".codegraph");
    try {
      // 已存在且指向正确则不动；否则（被 clean 删除、或是目录/错误链接）重建。
      const st = lstatSync(link);
      if (!st.isSymbolicLink()) rmSync(link, { recursive: true, force: true });
    } catch {
      // 不存在，建软链：src/.codegraph -> ../codegraph
      symlinkSync(join("..", "codegraph"), link, "dir");
    }
    if (!existsSync(join(dataDir, "codegraph.db"))) {
      process.stderr.write(`[stage] 首次建立 codegraph 索引（仅一次，后续审查增量更新）…\n`);
      runCodeGraph(src, ["init", "-f"], 180_000);
    } else {
      runCodeGraph(src, ["sync", "-q"], 60_000);
    }
    process.stderr.write(`[stage] codegraph 索引就绪\n`);
    return true;
  } catch (err) {
    process.stderr.write(`[warn] codegraph 索引不可用（将使用 grep/read 检索）：${(err?.message || err).split("\n")[0]}\n`);
    return false;
  }
}

// 同步执行 codegraph 子命令，关闭守护进程/自动更新/遥测/prompt hook 等 CI 不需要的行为。
function runCodeGraph(cwd, args, timeoutMs) {
  const r = spawnSync("codegraph", args, {
    cwd,
    encoding: "utf8",
    timeout: timeoutMs,
    env: {
      ...process.env,
      CODEGRAPH_NO_DAEMON: "1",
      CODEGRAPH_NO_WATCH: "1",
      CODEGRAPH_NO_UPDATE_CHECK: "1",
      CODEGRAPH_NO_PROMPT_HOOK: "1",
      CODEGRAPH_NO_WATCHDOG: "1",
      CODEGRAPH_NO_INSTALL_REFRESH: "1",
      NO_COLOR: "1",
    },
    maxBuffer: 64 * 1024 * 1024,
  });
  if (r.error) throw r.error;
  if (r.status !== 0) {
    throw new Error(`codegraph ${args.join(" ")} 退出码 ${r.status}：${(r.stderr || r.stdout || "").trim().slice(0, 300)}`);
  }
  return (r.stdout || "").trim();
}

// 读取目标 commit 的真正提交者（git 元数据里的作者，而非推送者/触发者）。
// author 键用 email（同一人稳定唯一，便于服务端按人聚合评分），同时保留 name。
function readCommitAuthor(src, head) {
  try {
    const name = git(src, ["log", "-1", "--format=%an", head]).trim();
    const email = git(src, ["log", "-1", "--format=%ae", head]).trim();
    return { name, email };
  } catch {
    return { name: "", email: "" };
  }
}

// 把 agent 给出的文件路径归一化为仓库相对路径（去掉 a/ b/ 前缀、开头的 /、反斜杠）。
function normalizeRepoPath(p) {
  if (!p) return "";
  let s = String(p).replace(/\\/g, "/").trim();
  s = s.replace(/^([a-z]:)?\/+/i, ""); // 去掉盘符/绝对路径前缀
  s = s.replace(/^[ab]\//, "");        // git diff 惯用的 a/ b/ 前缀
  return s;
}

// 对 (file, line) 在 head 上跑 git blame，返回最后修改该行的作者 {name,email}。
// 用于把每条 finding 归属到真正写那行代码的人。失败时返回空（该 finding 记为未归属）。
function blameLine(src, head, file, line) {
  const rel = normalizeRepoPath(file);
  if (!rel || !line || line < 1) return { name: "", email: "" };
  try {
    const out = git(src, ["blame", "--line-porcelain", "-L", `${line},${line}`, head, "--", rel], {
      // blame 不应该触发任何钩子/交互
      GIT_TERMINAL_PROMPT: "0",
    });
    let name = "";
    let email = "";
    for (const l of out.split("\n")) {
      if (l.startsWith("author ")) name = l.slice("author ".length).trim();
      else if (l.startsWith("author-mail ")) {
        email = l.slice("author-mail ".length).trim().replace(/^<|>$/g, "");
      }
    }
    return { name, email };
  } catch {
    return { name: "", email: "" };
  }
}

// 列出 base..head 区间内的所有参与者（去重，按 email）。
function listParticipants(src, base, head) {
  const seen = new Map();
  try {
    const range = base ? `${base}..${head}` : head;
    const out = git(src, ["log", "--format=%an%x09%ae", range]);
    for (const line of out.split("\n")) {
      if (!line) continue;
      const tab = line.indexOf("\t");
      const name = tab >= 0 ? line.slice(0, tab) : "";
      const email = (tab >= 0 ? line.slice(tab + 1) : "").trim().toLowerCase();
      if (email && !seen.has(email)) seen.set(email, name.trim());
    }
  } catch {
    /* 忽略：参与者列表缺失只是少拆几份报告 */
  }
  return [...seen.entries()].map(([email, name]) => ({ name, email }));
}

// 按作者汇总 base..head 内的增删行与改动文件数（用于每个作者报告的统计数字）。
function authorDiffStats(src, base, head) {
  const stats = new Map();
  try {
    const range = base ? `${base}..${head}` : head;
    // commit:<email> 行标记作者，随后是该提交的 numstat 行。
    const out = git(src, ["log", "--format=commit:%ae", "--numstat", range]);
    let cur = "";
    for (const line of out.split("\n")) {
      if (line.startsWith("commit:")) {
        cur = line.slice("commit:".length).trim().toLowerCase();
        if (cur && !stats.has(cur)) stats.set(cur, { additions: 0, deletions: 0, files: new Set() });
        continue;
      }
      if (!cur || !line.trim()) continue;
      const parts = line.split("\t");
      if (parts.length < 3) continue;
      const add = Number(parts[0]);
      const del = Number(parts[1]);
      const file = parts[2];
      const s = stats.get(cur);
      if (!s) continue;
      if (Number.isFinite(add)) s.additions += add;
      if (Number.isFinite(del)) s.deletions += del;
      if (file) s.files.add(file);
    }
  } catch {
    /* 忽略 */
  }
  const out = {};
  for (const [email, s] of stats.entries()) {
    out[email] = { additions: s.additions, deletions: s.deletions, files_changed: s.files.size };
  }
  return out;
}

// 判断文件是否属于「不值得逐行审查」的低价值改动：依赖锁、构建产物、压缩/二进制、
// 生成代码等。这些文件会计入统计和文件清单，但只在清单里打 [generated/lock] 标签，
// 提示 agent 不必花预算逐行查看（除非有可疑点）。
function isLowValueFile(path) {
  const p = path.toLowerCase();
  if (/(^|\/)(node_modules|vendor|dist|build|out|target|__pycache__|\.git|coverage|\.next|\.nuxt)\//.test(p)) return true;
  if (/(^|\/)(package-lock\.json|pnpm-lock\.yaml|yarn\.lock|composer\.lock|go\.sum|cargo\.lock|gemfile\.lock|poetry\.lock)$/.test(p)) return true;
  if (/\.(min\.(js|css)|map|lock|sum)$/.test(p)) return true;
  if (/\.(png|jpe?g|gif|svg|ico|webp|bmp|pdf|zip|tar|gz|tgz|rar|7z|woff2?|ttf|eot|mp[34]|webm|wasm|class|jar|so|dylib|dll|exe)$/.test(p)) return true;
  return false;
}

// 计算本次审查范围内的提交区间与文件改动统计（不生成补丁文本）。
// 适配层绝不把 diff 内容塞进 prompt：只给 base/head 两个提交与「+/- 行数 路径」的
// 文件清单作为导航，具体改动由 agent 用自己的 bash/read/grep 工具逐文件查看，
// 避免上万行 diff 一次性灌入上下文导致模型零输出/失败。
function diffForReview(src, input) {
  const head = input.commit_sha || git(src, ["rev-parse", "HEAD"]);
  let base = input.base_sha || "";
  if (!base) {
    // 无 base 时取目标提交的第一父提交；首个提交则与空树比较。
    const parents = git(src, ["rev-list", "--parents", "-n", "1", head]).split(/\s+/);
    base = parents.length > 1 ? parents[1] : "4b825dc642cb6eb9a060e54bf8d69288fbee4904";
  }
  const commitAuthor = readCommitAuthor(src, head);
  const numstat = git(src, ["diff", "--numstat", base, head]);
  const files = [];
  let filesChanged = 0, additions = 0, deletions = 0;
  for (const line of numstat.split("\n")) {
    if (!line.trim()) continue;
    const parts = line.split("\t");
    const a = parts[0], d = parts[1], path = parts.slice(2).join("\t"); // 含重命名时 path 可能带 tab
    filesChanged++;
    const add = a === "-" ? 0 : Number(a) || 0;
    const del = d === "-" ? 0 : Number(d) || 0;
    additions += add; deletions += del;
    files.push({ path, add, del, binary: a === "-" });
  }

  // 文件清单（含每个文件 +/-），完整提供，作为 agent 自查时的导航地图；
  // 这只是路径与行数的元数据，不是代码内容，体积很小。
  const fileList = files.map((f) => {
    const tag = f.binary ? " [binary]" : isLowValueFile(f.path) ? " [generated/lock]" : "";
    return `${f.add.toString().padStart(6)} ${f.del.toString().padStart(6)}  ${f.path}${tag}`;
  }).join("\n");

  return { head, base, filesChanged, additions, deletions, fileList, commitAuthor };
}

// ---------- Pi 模型配置（每个模型 profile 来自平台 input.llm）----------
function writeModelConfig(home, input) {
  const llm = input.llm;
  if (!llm?.base_url || !llm?.model || !llm?.api_key) {
    throw new Error("input.json 缺少 llm.base_url/model/api_key，请先在「设置 → AI 模型」配置默认模型");
  }
  const models = {
    providers: {
      aicr: {
        baseUrl: llm.base_url,
        api: "openai-completions",
        compat: { supportsDeveloperRole: false, supportsReasoningEffort: false },
        models: [{
          id: llm.model,
          name: llm.model,
          reasoning: false,
          input: ["text"],
          contextWindow: llm.context_window || 64000,
          maxTokens: llm.max_tokens || 4096,
          cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
        }],
      },
    },
  };
  mkdirSync(home, { recursive: true });
  writeFileSync(join(home, "models.json"), JSON.stringify(models, null, 2), { mode: 0o600 });
}

// ---------- 主流程 ----------
async function main() {
  const opts = parseArgs(process.argv);
  const input = JSON.parse(readFileSync(opts.input, "utf8"));
  const workdir = resolve(opts.workdir);
  mkdirSync(workdir, { recursive: true });

  const t0 = Date.now();
  process.stderr.write(`[stage] 准备仓库（clone/fetch 到目标提交）…\n`);
  const src = prepareRepo(workdir, input);
  // 准备 codegraph 索引（失败不阻断，降级为 grep/read）；让 agent 精确定位符号与调用关系，减少盲目检索。
  const codegraphReady = prepareCodeGraph(workdir, src);
  process.stderr.write(`[stage] 计算 diff 并读取提交作者…\n`);
  const diff = diffForReview(src, input);
  // 审查工作量上限：只对比 base..head 这一个提交区间，但文件可能很多，必须限定深入查看的
  // 文件数与工具调用数，避免 AI 逐个文件无限看下去导致审查过久。
  // 文件数：最少 5 个，最多 15 个，区间内随总文件数小幅增长（取 log 缩放，避免线性膨胀）。
  const reviewableFiles = Math.max(0, diff.filesChanged); // 文件清单里含 binary/lock，预算只约束「深入查看」
  const fileBudget = Math.max(5, Math.min(15,
    5 + Math.round(Math.log2(reviewableFiles + 1) * 1.5)));
  // 工具调用预算：每文件约 4~6 次（diff + read + grep 等），加上少量范围探测。
  const toolBudget = fileBudget * 5 + 6;
  const budget = { files: fileBudget, toolCalls: toolBudget };
  process.stderr.write(`[stage] 提交区间就绪：${diff.base.slice(0,10)}..${diff.head.slice(0,10)}，${diff.filesChanged} 个文件，+${diff.additions} / -${diff.deletions}；AI 将自行逐文件查看（预算：最多深入 ${budget.files} 个文件、约 ${budget.toolCalls} 次工具调用）…\n`);

  // 独立的 Pi 配置/会话目录，不污染 /data，也不持久化会话。
  const home = join(workdir, ".pi-home");
  rmSync(home, { recursive: true, force: true });
  mkdirSync(home, { recursive: true });
  writeModelConfig(home, input);

  process.env.PI_SKIP_VERSION_CHECK = "1";
  process.env.PI_OFFLINE = "1";
  process.env.PI_TELEMETRY = "0";
  // 让被 agent 调用的 git 也不走钩子。
  process.env.GIT_CONFIG_COUNT = "1";
  process.env.GIT_CONFIG_KEY_0 = "core.hooksPath";
  process.env.GIT_CONFIG_VALUE_0 = "/dev/null";

  const modelRuntime = await ModelRuntime.create({
    authPath: join(home, "auth.json"),
    modelsPath: join(home, "models.json"),
  });
  await modelRuntime.setRuntimeApiKey("aicr", input.llm.api_key);

  const model = modelRuntime.getModel("aicr", input.llm.model);
  if (!model) throw new Error(`模型未在自定义 provider 中注册：${input.llm.model}`);

  let submitted = null;

  const resourceLoader = new DefaultResourceLoader({
    cwd: src,
    agentDir: home,
    noContextFiles: true,   // 不加载被审仓库内的 AGENTS.md / CLAUDE.md
    additionalSkillPaths: [join(__dirname, "skills")],
    systemPrompt: [
      "你是一名严谨的资深代码审查员，运行在 CI 环境中，对一次代码提交做只读审查。",
      "你不能修改代码，只能阅读、搜索、运行只读的 shell 命令（如 git diff / git show）。",
      "",
      "工作目录就是被审仓库，当前已 checkout 到待审提交。",
      "审查必须逐步进行：任务只会告诉你 base 与 head 两个提交以及文件清单，不会把 diff 内容直接给你。",
      "你需要自己用 `git diff <base> <head> -- <path>` 逐文件查看改动，用 read/grep 阅读上下文，",
      "绝不要一次性 `git diff` 整个范围或用 cat 读取大文件，以免上下文爆炸。",
      "请遵循 code-review 技能（/skill:code-review）的流程，",
      "最后必须调用 submit_report 工具提交一份结构化报告。",
      "",
      "硬性要求：",
      "1. 必须且只能调用一次 submit_report，调用后立即结束，不要做其他操作。",
      "2. findings 的 category 必须取自给定维度的 key（dimension key），否则该问题不计入扣分。",
      "3. severity 只能是 critical | high | medium | low | info；info 不扣分。",
      "4. 评分 0-100，按每个维度的「评分标准描述」独立打分；没有问题给高分，问题要在 rationale 中说明。",
      "5. 只报告真实、可定位的问题；每个 finding 的 file_path/line 必须准确。",
      "6. summary 用中文，简洁概括本次改动的质量与主要问题。",
    ].join("\n"),
    extensionFactories: [(pi) => {
      pi.registerTool({
        name: "submit_report",
        label: "提交审查报告",
        description: "完成分析后调用，提交本次代码审查的结构化报告。只能调用一次。",
        parameters: Type.Object({
          summary: Type.String({ description: "中文摘要，概述改动质量与主要问题" }),
          dimensions: Type.Record(Type.String(), Type.Object({
            score: Type.Integer({ minimum: 0, maximum: 100, description: "该维度 0-100 分" }),
            rationale: Type.String({ description: "打分理由" }),
          })),
          findings: Type.Array(Type.Object({
            rule_id: Type.String({ description: "简短规则标识，如 security/sql-injection" }),
            severity: Type.Union([
              Type.Literal("critical"), Type.Literal("high"),
              Type.Literal("medium"), Type.Literal("low"), Type.Literal("info"),
            ]),
            category: Type.String({ description: "所属维度 key，必须命中 dimensions 中的某个 key" }),
            file_path: Type.String(),
            line: Type.Integer({ minimum: 1 }),
            line_end: Type.Optional(Type.Integer({ minimum: 1 })),
            title: Type.String(),
            message: Type.String({ description: "问题说明" }),
            snippet: Type.Optional(Type.String()),
            suggestion: Type.Optional(Type.String({ description: "修改建议" })),
            confidence: Type.Optional(Type.Union([
              Type.Literal("high"), Type.Literal("medium"), Type.Literal("low"),
            ])),
          })),
          strengths: Type.Optional(Type.Array(Type.String())),
          risks: Type.Optional(Type.Array(Type.String())),
          tokens_used: Type.Optional(Type.Integer()),
        }),
        async execute(_id, params) {
          submitted = params;
          return {
            content: [{ type: "text", text: "报告已接收，审查结束。" }],
            details: {},
          };
        },
      });
    }],
  });
  await resourceLoader.reload();

  const settingsManager = SettingsManager.inMemory({
    compaction: { enabled: false },
    retry: { enabled: true, maxRetries: 2 },
  });

  const { session } = await createAgentSession({
    cwd: src,
    agentDir: home,
    model,
    thinkingLevel: "off",
    modelRuntime,
    resourceLoader,
    settingsManager,
    // 只读工具集 + 自定义 submit_report（必须显式列出，否则工具不会暴露给模型）。
    tools: ["read", "bash", "grep", "find", "ls", "submit_report"],
    sessionManager: SessionManager.inMemory(src),
  });

  const dimensions = (input.config?.dimensions ?? []);
  const dimText = dimensions.length
    ? dimensions.map((d) =>
        `- key=${d.key}｜名称：${d.label}｜权重：${d.weight}｜评分标准：${d.description}`).join("\n")
    : "（未提供维度配置，请按通用的架构/质量/安全/可维护四个维度，key 分别用 architecture/quality/security/maintainability）";

  const prompt = [
    `# 审查任务`,
    ``,
    `- 仓库：${input.clone_url}`,
    `- 待审提交：${diff.head}`,
    `- 基线：${diff.base}`,
    input.pr ? `- PR：#${input.pr.number} ${input.pr.title}（${input.pr.url}）` : "",
    ``,
    `## 评分维度（findings.category 必须使用这些 key）`,
    dimText,
    ``,
    `## 文件改动清单（共 ${diff.filesChanged} 个文件，+${diff.additions} / -${diff.deletions}）`,
    "```",
    diff.fileList,
    "```",
    ``,
    `## 如何查看改动（重要）`,
    `上面只有文件路径和增删行数，**没有把代码内容贴给你**——这是刻意为之：`,
    `审查必须一点一点进行，绝不要一次性读取整个 diff。请用你自己的工具逐文件查看：`,
    `- 看某个文件改了什么：\`git diff ${diff.base} ${diff.head} -- <path>\``,
    `- 看提交概览：\`git show --stat ${diff.head}\``,
    `- 用 read 读取改动周边的上下文、grep 查找调用点，确认问题是否真实。`,
    `- **禁止**直接执行不带路径的 \`git diff ${diff.base} ${diff.head}\` 或 \`git show ${diff.head}\`：那会一次性吐出全部改动。`,
    codegraphReady
      ? [
          ``,
          `## 用 codegraph 精确定位（优先，可大幅减少检索）`,
          `仓库已建好 codegraph 索引。当你需要理解一个函数/方法/类的来源、调用方、实现或影响范围时，**优先用 codegraph，不要先 grep 全库**：`,
          `- \`codegraph explore "<符号或问题>"\`：一次返回相关符号的逐行源码 + 它们之间的调用路径，相当于一次精准的多文件检索。`,
          `- \`codegraph node "<符号名>"\`：看某个符号的源码及其调用者/被调用者。`,
          `- \`codegraph callers "<符号>"\`：只查谁调用了它。`,
          `典型流程：在某个改动文件里看到关键调用 → \`codegraph node/explore\` 定位定义和调用方 → 只对确需细看的少数文件用 \`git diff\`/\`read\`。codegraph 已给出的源码块不要再重复 read。`,
        ].join("\n")
      : "",
    ``,
    `## 工作量上限（必须遵守，避免审查过久）`,
    `本次审查你**最多深入查看 ${budget.files} 个源码文件**、**最多发起约 ${budget.toolCalls} 次工具调用**，到上限就必须收束并提交报告。`,
    `- 文件数很少（≤${budget.files}）时全部覆盖；文件很多时按下面优先级抽样，不要试图看完所有文件。`,
    `- 先花少量工具调用确定范围（1~2 次 \`git diff --stat\` / 按目录看），再把预算花在最值得看的文件上。`,
    `- 选择文件的优先级：安全/鉴权/加密相关 > 核心业务逻辑与数据处理 > SQL/事务/并发 > 接口契约/配置 > 其它。`,
    `- 标记 [generated/lock] 的依赖锁、构建产物、压缩文件不计入预算，也不要逐行看，summary 里点一句即可；[binary] 直接跳过。`,
    `- 没看的文件不要编造问题。findings 必须来自你实际读过的代码，file_path/line 准确。`,
    `- 即使有文件没看完，只要已覆盖主要风险点，就提交报告并在 summary 里说明审查范围（如"抽样审查了 N/M 个文件"）。`,
    ``,
    `审查步骤：`,
    `1. \`git diff --stat ${diff.base} ${diff.head}\` 掌握全貌。`,
    `2. 按优先级挑出最多 ${budget.files} 个文件，逐个 \`git diff ... -- <path>\`，必要时 read 上下文 / grep 调用点。`,
    `3. 工具调用接近 ${budget.toolCalls} 次或已看够 ${budget.files} 个文件时停止探索，调用 submit_report。`,
    ``,
    `请按 code-review 技能完成审查，并调用 submit_report 提交报告。`,
  ].filter(Boolean).join("\n");

  let lastDelta = "";
  // 记录会话过程中的底层错误（API 报错、上下文溢出压缩失败、自动重试等），
  // 这些事件之前没被订阅，导致失败时只能看到「最后输出为空」，无法定位根因。
  const sessionErrors = [];
  const DEBUG = process.env.PI_AGENT_DEBUG === "1";
  // 工具调用计数：对照 prompt 里给的软预算，超预算时在日志里提示，便于观察审查是否失控。
  let toolCalls = 0;
  let budgetAborted = false;
  const hardToolLimit = budget.toolCalls * 2;
  // 把工具调用格式化成简洁一行（如 `read src/a.ts`、`bash git diff --stat`），便于实时查看进度。
  function toolSummary(name, args) {
    try {
      const a = args || {};
      if (name === "bash") return (a.command || "").replace(/\s+/g, " ").slice(0, 160);
      if (name === "read") return a.path || a.filePath || "";
      if (name === "grep") return `${a.pattern ? a.pattern + " " : ""}${a.path || ""}`;
      if (name === "find") return a.path || a.startPath || "";
      if (name === "ls") return a.path || "";
      if (name === "submit_report") return "";
      return JSON.stringify(a).slice(0, 120);
    } catch {
      return "";
    }
  }
  // 从工具执行结果（AgentToolResult）提取纯文本输出：优先 content 里的 text 块，
  // 回退 result 本身（字符串/JSON）。用于回显命令执行结果。
  function toolResultText(result) {    try {
      if (result == null) return "";
      if (typeof result === "string") return result.replace(/\s+$/g, "");
      if (Array.isArray(result.content)) {
        const parts = result.content
          .filter((c) => c && c.type === "text" && typeof c.text === "string")
          .map((c) => c.text.replace(/\s+$/g, ""));
        if (parts.length > 0) return parts.filter(Boolean).join("\n");
      }
      if (typeof result === "object") {
        // 兜底：部分工具直接把 {output/stdout/text} 放在 result 上。
        const t = result.output || result.stdout || result.text || result.message;
        if (typeof t === "string") return t.replace(/\s+$/g, "");
      }
    } catch {
      // ignore
    }
    return "";
  }
  // 判断 bash 工具的非零退出是否只是「搜索/查找无命中」这类无害情况。
  // grep/rg/find/ls 等在没找到匹配或路径不存在时会返回非零，但这属于正常探索，
  // 不应在日志里标成红色「执行失败」淹没真正的错误。
  // 返回 true 表示「无害，降级为普通提示」；false 表示真错误，保留 warn。
  function isBenignBashMiss(args, output) {
    try {
      const cmd = String(args?.command || "");
      const argv = cmd.trim().split(/\s+/);
      const head = argv[0];
      const searchCmds = new Set(["grep", "egrep", "fgrep", "rg", "find", "ls", "test", "[", "git"]);
      if (!searchCmds.has(head)) return false;
      if (head === "git") {
        const ok = new Set(["grep", "log", "show-ref", "rev-parse", "diff", "ls-files", "branch", "tag", "cat-file"]);
        if (!ok.has(argv[1] || "")) return false;
      }
      const text = String(output || "").toLowerCase();
      // 退出码 1（无匹配/无输出）且输出里没有真正的错误措辞，视为正常探索。
      if (text.includes("exited with code 1") || text === "" || text === "(no output)") {
        const realErr = /no such file|cannot |can't |not found|permission denied|usage:|unknown option|unrecognized|syntax error|is a directory|command not found|fatal:/;
        return !realErr.test(text);
      }
      return false;
    } catch {
      return false;
    }
  }
  session.subscribe((event) => {
    if (event.type === "message_update" && event.assistantMessageEvent?.type === "text_delta") {
      lastDelta += event.assistantMessageEvent.delta;
      if (DEBUG) process.stderr.write(event.assistantMessageEvent.delta);
    } else if (event.type === "tool_execution_start") {
      // 始终输出工具调用进度（submit_report 单独提示）。
      if (event.toolName === "submit_report") {
        process.stderr.write(`[stage] AI 正在提交审查报告…\n`);
      } else {
        toolCalls++;
        process.stderr.write(`[tool] ${event.toolName} ${toolSummary(event.toolName, event.args)}\n`);
        // 硬性安全网：探索工具调用达到硬上限（软预算的 2 倍）仍未提交，中断当前回合，
        // 由外层 nudge 强制用已掌握的信息提交，避免 AI 无限探索导致审查过久。
        if (toolCalls >= hardToolLimit && !submitted && !budgetAborted) {
          budgetAborted = true;
          process.stderr.write(`[warn] 工具调用已达硬上限 ${hardToolLimit}，强制收束并提交报告\n`);
          session.abort().catch(() => {});
        }
      }
      if (DEBUG) process.stderr.write(`\n[tool→] ${event.toolName} ${JSON.stringify(event.args).slice(0, 300)}\n`);
    } else if (event.type === "tool_execution_end") {
      if (event.toolName === "submit_report") {
        // 收到报告即结束，避免模型继续空转。
        session.abort().catch(() => {});
      } else {
        // 回显工具执行结果（命令的真实输出），截断后按行缩进，形成「命令→结果」滚动。
        const out = toolResultText(event.result);
        if (event.isError) {
          // 搜索类命令无命中退出码 1 是正常现象，不标红，仅给一行提示，避免淹没真错误。
          if (event.toolName === "bash" && isBenignBashMiss(event.args, out)) {
            process.stderr.write(`[info] 搜索无命中（退出码 1，无匹配）\n`);
          } else {
            process.stderr.write(`[warn] 工具 ${event.toolName} 执行失败\n`);
          }
        }
        if (out) {
          const MAX = 4000; // 单条结果最多回显 4KB，避免大文件读取刷屏/撑爆日志表
          const body = out.length > MAX ? out.slice(0, MAX) + "\n…[结果已截断]" : out;
          for (const ln of body.split("\n")) process.stderr.write(`  │ ${ln}\n`);
        }
      }
      if (DEBUG) process.stderr.write(`[tool←] ${event.toolName} error=${event.isError}\n`);
    } else if (event.type === "auto_retry_start") {
      // 底层 LLM 调用出错（限流/过载/5xx）正在自动重试
      const msg = `[warn] LLM 调用失败，自动重试（${event.attempt}/${event.maxAttempts}）：${event.errorMessage}`;
      process.stderr.write(msg + "\n");
      sessionErrors.push(event.errorMessage);
    } else if (event.type === "auto_retry_end" && !event.success) {
      const msg = `[warn] LLM 自动重试结束，仍未成功：${event.finalError || "未知错误"}`;
      process.stderr.write(msg + "\n");
      sessionErrors.push(event.finalError || "unknown");
    } else if (event.type === "compaction_start") {
      process.stderr.write(`[warn] 上下文较长，正在压缩（reason=${event.reason}）…\n`);
    } else if (event.type === "compaction_end" && event.errorMessage) {
      process.stderr.write(`[warn] 上下文压缩失败：${event.errorMessage}\n`);
      sessionErrors.push(event.errorMessage);
    } else if (event.type === "warning") {
      const m = event.message || event.warning || "";
      if (m) process.stderr.write(`[warn] ${m}\n`);
    } else if (event.type === "error" || event.type === "session_error" || event.type === "request_error") {
      // 兜底：不同 SDK 版本对 LLM 请求失败/流式中断的事件名不统一，
      // 之前只监听 auto_retry_* 会漏掉「首轮就失败、未触发重试」的情况，
      // 表现为「模型未产生任何文本输出」却看不到根因。
      const msg = event.error?.message || event.message || event.errorMessage || JSON.stringify(event);
      if (msg) {
        process.stderr.write(`[error] 会话底层错误（${event.type}）：${String(msg).slice(0, 800)}\n`);
        sessionErrors.push(String(msg));
      }
    }
  });

  // 模型有时在工具探索后结束回合却忘了提交报告；回合结束后若未提交，
  // 强提醒一次再跑一轮，最多两轮，避免偶发的"未提交"失败。
  // 若因触达工具调用硬上限被 abort，直接进入收束提示。
  const nudges = [
    "你还没有调用 submit_report。请立即根据已分析的内容调用 submit_report 提交结构化报告，不要再做其他探索。",
    "仍未收到 submit_report。请现在就调用它，summary/dimensions/findings 按你已掌握的信息填写（可空数组），这是本次任务的唯一结束方式。",
  ];
  if (budgetAborted) {
    nudges.unshift("工具调用已达上限，必须停止探索。立即用你已经查看过的文件和信息调用 submit_report 提交报告，不要再调用任何 bash/read/grep/find/ls；没看全的文件不要编造，在 summary 中说明本次为抽样审查。");
  }
  for (let round = 0; round <= nudges.length && !submitted; round++) {
    try {
      await session.prompt(round === 0 ? prompt : nudges[round - 1]);
    } catch (err) {
      // abort 会让 prompt reject；只要已拿到报告就视为正常。
      if (submitted) break;
      // 记录真实的 reject 原因（首轮 API 失败曾被这里吞掉，只露出"未产生文本"）。
      const reason = err?.message || String(err);
      if (reason && !/abort/i.test(reason)) {
        process.stderr.write(`[warn] 第 ${round + 1} 轮 prompt 失败：${reason.slice(0, 800)}\n`);
        sessionErrors.push(reason);
      }
      if (round >= nudges.length) throw err;
    }
  }
  session.dispose();

  if (!submitted) {
    const tail = lastDelta.trim().slice(-500) || "(模型未产生任何文本输出)";
    const errs = sessionErrors.length ? `\n会话期间的底层错误：\n- ${[...new Set(sessionErrors)].slice(-5).join("\n- ")}` : "";
    throw new Error(`agent 未调用 submit_report，未能生成审查报告。最后输出：${tail}${errs}`);
  }

  // 多作者归属：AI 只审一次整个区间。现在用 git blame 把每条 finding 定位到最后修改
  // 该行的作者（email 为稳定键），服务端据此为每位参与者各生成一份报告与评分。
  const findings = submitted.findings ?? [];
  if (findings.length > 0) {
    process.stderr.write(`[stage] 用 git blame 归属 ${findings.length} 条问题到作者…\n`);
    for (const f of findings) {
      const a = blameLine(src, diff.head, f.file_path, f.line);
      f.author = a.email ? a.email.trim().toLowerCase() : "";
      f.author_name = a.name || "";
    }
  }
  const participants = listParticipants(src, diff.base, diff.head);
  const authorStats = authorDiffStats(src, diff.base, diff.head);
  if (participants.length > 0) {
    process.stderr.write(`[stage] 本次区间涉及 ${participants.length} 位作者：${participants.map(p => p.name || p.email).join(", ")}\n`);
  }

  const report = {
    summary: submitted.summary,
    dimensions: submitted.dimensions,
    findings,
    strengths: submitted.strengths ?? [],
    risks: submitted.risks ?? [],
    stats: {
      files_changed: diff.filesChanged,
      additions: diff.additions,
      deletions: diff.deletions,
    },
    tokens_used: submitted.tokens_used ?? 0,
    // 适配层不再内联/截断 diff，agent 可自行查看完整提交区间，故恒为 false。
    truncated: false,
    // 被审 commit 的真正作者（取自 git 元数据），供服务端按人聚合评分。
    commit_author: {
      name: diff.commitAuthor.name || "",
      email: diff.commitAuthor.email || "",
    },
    // 多作者拆分所需：区间参与者 + 每条 finding 的 blame 作者 + 各作者增删统计。
    participants,
    author_stats: authorStats,
  };
  writeFileSync(opts.output, JSON.stringify(report, null, 2), { mode: 0o600 });

  const ms = Date.now() - t0;
  process.stderr.write(`[pi-agent] review done in ${ms}ms, findings=${report.findings.length}\n`);
}

main().catch((err) => {
  process.stderr.write(`[pi-agent] ${err?.stack || err?.message || err}\n`);
  process.exit(1);
});
