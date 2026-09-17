package logging

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// DefaultDir 是日志目录，DefaultFileName 是客户端日志文件名。
const (
	DefaultDir      = "logs"
	DefaultFileName = "client.log"
)

// DefaultPath 返回默认日志文件路径，供界面直接打开日志查看详细错误。
func DefaultPath() string { return filepath.Join(DefaultDir, DefaultFileName) }

// FilePath 返回指定日志目录下的客户端日志文件路径。
func FilePath(dir string) string { return filepath.Join(dir, DefaultFileName) }

// New creates the application logger and ensures the log directory exists.
func New(dir string) (*log.Logger, io.Closer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(FilePath(dir), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return log.New(file, "", log.LstdFlags), file, nil
}
