package aicr

import "embed"

// WebFS 嵌入前端构建产物。
// Docker 构建会先跑 vite build 生成 web/dist；
// 本地若 web/dist 下只有 .gitkeep，占位页保证 go build 不失败。
//
//go:embed all:web/dist
var WebFS embed.FS
