// Package ffmpeg 解析配置中的 FFmpeg 可执行文件路径。
package ffmpeg

import (
	"fmt"
	"os/exec"
	"strings"
)

// Resolve 校验 FFmpeg 可用并返回可执行文件路径。configured 为空时在 PATH 中查找 ffmpeg。
func Resolve(configured string) (string, error) {
	bin := strings.TrimSpace(configured)
	if bin == "" {
		bin = "ffmpeg"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", fmt.Errorf("未找到 FFmpeg（ffmpeg_path=%q）: %w", configured, err)
	}
	return path, nil
}
