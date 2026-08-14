package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// CLI 通过子进程调用 Pi Agent 执行审查。
// 约定：把任务 spec 写入工作目录的 input.json，执行
//   <bin> run --input input.json --output report.json [--workdir <dir>]
// Pi Agent 完成后从 report.json 读取结构化报告。
type CLI struct {
	bin     string
	dataDir string
	log     *zap.Logger
}

func NewCLI(bin, dataDir string, log *zap.Logger) *CLI {
	return &CLI{bin: bin, dataDir: dataDir, log: log}
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
	workDir := filepath.Join(c.dataDir, "work", fmt.Sprintf("review-%d-%d", in.RepoID, time.Now().UnixNano()))
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir workdir: %w", err)
	}
	defer os.RemoveAll(workDir)

	inputPath := filepath.Join(workDir, "input.json")
	outputPath := filepath.Join(workDir, "report.json")

	spec, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(inputPath, spec, 0o600); err != nil {
		return nil, err
	}

	// 整个审查最长 10 分钟
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
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
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	c.log.Info("running pi-agent", zap.String("bin", c.bin), zap.Int64("repo_id", in.RepoID), zap.String("commit", in.CommitSHA))
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pi-agent run failed: %w; stderr: %s", err, stderr.String())
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
