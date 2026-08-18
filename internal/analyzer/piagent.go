package analyzer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CLI 通过子进程调用 Pi Agent 执行审查。
// 约定：把任务 spec 写入工作目录的 input.json，执行
//   <bin> run --input input.json --output report.json [--workdir <dir>]
// Pi Agent 完成后从 report.json 读取结构化报告。
//
// 工作目录使用持久化的 work/repo-<repoID>（不在每次审查后删除），
// Pi Agent 在已有目录上 fetch 增量、没有则 clone。同仓库并发审查通过
// per-repo mutex 串行化（不同仓库仍并行）。
type CLI struct {
	bin       string
	dataDir   string
	log       *zap.Logger
	repoLocks sync.Map // int64 repoID -> *sync.Mutex
}

func NewCLI(bin, dataDir string, log *zap.Logger) *CLI {
	return &CLI{bin: bin, dataDir: dataDir, log: log}
}

// repoMu 返回某仓库的进程内互斥锁（同仓库任务串行）。
func (c *CLI) repoMu(repoID int64) *sync.Mutex {
	v, _ := c.repoLocks.LoadOrStore(repoID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// writeSSHKey 将 SSH 私钥写入 dataDir/keys/<repoID>（0600），返回路径供 GIT_SSH_COMMAND 使用。
// 文件持久保留（跨审查复用，且不在随 workdir 清理时删除）。同一仓库的凭据更新会覆盖该文件。
func (c *CLI) writeSSHKey(repoID int64, pem string) (string, error) {
	dir := filepath.Join(c.dataDir, "keys")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir keys: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("repo-%d", repoID))
	if err := os.WriteFile(path, []byte(pem), 0o600); err != nil {
		return "", fmt.Errorf("write ssh key: %w", err)
	}
	return path, nil
}

func (c *CLI) Run(ctx context.Context, in PiAgentInput) (*PiAgentReport, error) {
	// 同仓库任务串行：持久 workdir 不能被两个 git/Pi Agent 进程同时操作。
	mu := c.repoMu(in.RepoID)
	mu.Lock()
	defer mu.Unlock()

	workDir := filepath.Join(c.dataDir, "work", "repo-"+strconv.FormatInt(in.RepoID, 10))
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir workdir: %w", err)
	}
	// 不删除 workDir：Pi Agent 在该目录上增量 fetch，没有才 clone。

	inputPath := filepath.Join(workDir, "input.json")
	outputPath := filepath.Join(workDir, "report.json")

	spec, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(inputPath, spec, 0o600); err != nil {
		return nil, err
	}

	// 整个审查最长 25 分钟（大仓库 + 多次工具调用需要更宽裕；adapter 内部对单次
	// LLM 请求另设 5 分钟硬超时，避免单个挂起请求拖满整个窗口）。
	runCtx, cancel := context.WithTimeout(ctx, 25*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(runCtx, c.bin, "run",
		"--input", inputPath,
		"--output", outputPath,
		"--workdir", workDir,
	)
	env := append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		// 禁止执行仓库内 git hooks
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.hooksPath",
		"GIT_CONFIG_VALUE_0=/dev/null",
	)
	// SSH 凭据：把私钥写入持久目录（不随每次 workdir 删除），通过 GIT_SSH_COMMAND 注入。
	// 注意：仅当 clone_url 为 SSH 形式（git@host:org/repo.git）时 ssh 才会被使用。
	if in.SSHPrivateKey != "" {
		keyPath, err := c.writeSSHKey(in.RepoID, in.SSHPrivateKey)
		if err != nil {
			return nil, err
		}
		env = append(env, "GIT_SSH_COMMAND=ssh -i "+keyPath+
			" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes")
	}
	cmd.Env = env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	// 逐行捕获子进程输出：写日志回调（若 ctx 注入了 sink，则实时落库供前端查看）、
	// 同时记 zap 日志、并在 ring buffer 中保留最后 8KB 供失败时拼进 error。
	// stdout/stderr 必须并发读取，否则一个管道写满会阻塞子进程。
	sink := logSinkFromCtx(ctx)
	tail := newRingBuffer(8 << 10)
	scanPipe := func(r io.Reader, done chan<- struct{}) {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			tail.WriteString(line + "\n")
			c.log.Info("pi-agent", zap.String("line", line))
			if sink != nil {
				sink(line)
			}
		}
		close(done)
	}
	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})
	go scanPipe(stdout, stdoutDone)
	go scanPipe(stderr, stderrDone)

	c.log.Info("running pi-agent", zap.String("bin", c.bin), zap.Int64("repo_id", in.RepoID), zap.String("commit", in.CommitSHA))
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	waitErr := cmd.Wait()
	<-stdoutDone // 等所有输出读完
	<-stderrDone
	if waitErr != nil {
		return nil, fmt.Errorf("pi-agent run failed: %w; stderr: %s", waitErr, tail.String())
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("read report: %w", err)
	}
	var report PiAgentReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parse report: %w", err)
	}
	return &report, nil
}

// ringBuffer 是一个固定容量的字节环形缓冲，保留最近写入的内容，
// 用于在子进程失败时把最后的输出拼进 error。并发安全。
type ringBuffer struct {
	mu  sync.Mutex
	buf []byte
	cap int
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{cap: capacity}
}

func (r *ringBuffer) WriteString(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, s...)
	if len(r.buf) > r.cap {
		r.buf = r.buf[len(r.buf)-r.cap:]
	}
}

func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}
