package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultDir 是日志目录，DefaultFileName 是客户端日志文件名。
const (
	DefaultDir      = "logs"
	DefaultFileName = "client.log"
)

// Log 是业务代码使用的统一日志接口。具体输出目标由 Logger 实例按运行模式配置。
type Log interface {
	Info(string, ...any)
	Warning(string, ...any)
	Error(string, ...any)
}

// DefaultPath 返回默认日志文件路径，供界面直接打开日志查看详细错误。
func DefaultPath() string { return filepath.Join(DefaultDir, DefaultFileName) }

// FilePath 返回指定日志目录下的客户端日志文件路径。
func FilePath(dir string) string { return filepath.Join(dir, DefaultFileName) }

// New creates the application logger and ensures the log directory exists.
type Logger struct {
	file    *log.Logger
	closer  io.Closer
	mu      sync.RWMutex
	console io.Writer
	tui     func(string)
}

var defaultLogger struct {
	logger *Logger
}

// Default 返回当前进程复用的默认日志实例。
func Default() *Logger {
	if defaultLogger.logger == nil {
		logger, _, err := New(DefaultDir)
		if err == nil {
			defaultLogger.logger = logger
		}
	}
	return defaultLogger.logger
}

// Info 使用默认日志实例记录信息；日志初始化失败时静默跳过。
func Info(format string, args ...any) {
	if logger := Default(); logger != nil {
		logger.Info(format, args...)
	}
}

// SetConsole configures the process default logger to write to the console.
func SetConsole(writer io.Writer) {
	if logger := Default(); logger != nil {
		logger.SetConsole(writer)
	}
}

// SetTUISink configures the process default logger to send messages to the TUI.
func SetTUISink(sink func(string)) {
	if logger := Default(); logger != nil {
		logger.SetTUISink(sink)
	}
}

func New(dir string) (*Logger, io.Closer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(FilePath(dir), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	logger := &Logger{file: log.New(file, "", log.LstdFlags), closer: file}
	return logger, file, nil
}

// SetConsole configures a run-mode destination in addition to the log file.
func (l *Logger) SetConsole(writer io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.console = writer
	l.tui = nil
}

// SetTUISink configures a TUI destination in addition to the log file.
func (l *Logger) SetTUISink(sink func(string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tui = sink
}

func (l *Logger) Print(values ...any) { l.write(fmt.Sprint(values...)) }

func (l *Logger) Println(values ...any) { l.write(fmt.Sprintln(values...)) }

func (l *Logger) Printf(format string, args ...any) { l.write(fmt.Sprintf(format, args...)) }

func (l *Logger) Info(format string, args ...any) {
	l.writeLevel("INFO", format, args...)
}

func (l *Logger) Warning(format string, args ...any) {
	l.writeLevel("WARNING", format, args...)
}

func (l *Logger) Error(format string, args ...any) {
	l.writeLevel("ERROR", format, args...)
}

func (l *Logger) writeLevel(level, format string, args ...any) {
	l.write("[" + level + "] " + fmt.Sprintf(format, args...))
}

func (l *Logger) write(message string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.file.Print(message)
	if l.console != nil && l.tui == nil {
		consoleMessage := time.Now().Format("2006/01/02 15:04:05 ") + message
		if strings.HasSuffix(message, "\n") {
			fmt.Fprint(l.console, consoleMessage)
		} else {
			fmt.Fprintln(l.console, consoleMessage)
		}
	}
	if l.tui != nil {
		l.tui(message)
	}
}
