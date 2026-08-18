package analyzer

import (
	"context"
	"strings"
)

// LogSink 逐行接收审查执行过程日志（由 Pipeline 注入，CLI 捕获子进程输出后回调）。
type LogSink func(line string)

type logSinkKey struct{}

// withLogSink 把日志回调挂到 ctx，供底层 CLI.Run 取出使用。
func withLogSink(ctx context.Context, sink LogSink) context.Context {
	return context.WithValue(ctx, logSinkKey{}, sink)
}

// logSinkFromCtx 取出 ctx 中的日志回调；未设置时返回 nil（安全 no-op）。
func logSinkFromCtx(ctx context.Context) LogSink {
	if v, ok := ctx.Value(logSinkKey{}).(LogSink); ok {
		return v
	}
	return nil
}

// levelFor 按日志行内容粗分级别：[warn]/失败/error 等关键词决定 warn/error，其余 info。
func levelFor(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(line, "[warn]"):
		return "warn"
	case strings.Contains(line, "[pi-agent] Error") || strings.Contains(lower, "fatal") ||
		strings.Contains(line, "失败：") || strings.Contains(line, "失败:") ||
		strings.Contains(lower, "error:") || strings.Contains(lower, "panic"):
		return "error"
	default:
		return "info"
	}
}
