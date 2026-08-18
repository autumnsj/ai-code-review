package analyzer

import (
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// CleanupOldWorkDirs 删除超过 maxAge 未被访问的持久工作目录（work/repo-<id>）。
// 正常运行时工作目录持久保留供增量 fetch，仅在仓库长时间无审查后由这里保守回收。
// 在启动时调用一次即可。
func CleanupOldWorkDirs(dataDir string, maxAge time.Duration, log *zap.Logger) {
	root := filepath.Join(dataDir, "work")
	entries, err := os.ReadDir(root)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warn("read work dir for cleanup", zap.Error(err))
		}
		return
	}
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.RemoveAll(filepath.Join(root, e.Name())); err != nil {
				log.Warn("remove old work dir", zap.String("dir", e.Name()), zap.Error(err))
				continue
			}
			removed++
		}
	}
	if removed > 0 {
		log.Info("cleaned up old work dirs", zap.Int("removed", removed))
	}
}
