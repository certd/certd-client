package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// openLogFileWithSystemViewer 用系统默认程序打开日志文件，便于用户按菜单直接查看详细错误。
func openLogFileWithSystemViewer(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("解析日志文件路径失败: %w", err)
	}
	if err := ensureLogFile(absolute); err != nil {
		return err
	}
	command := logViewerCommand(runtime.GOOS, absolute)
	if err := command.Start(); err != nil {
		return fmt.Errorf("启动日志查看程序失败: %w", err)
	}
	// 查看程序应在客户端退出后继续可用，因此不等待其结束，只回收子进程句柄。
	go func() { _ = command.Wait() }()
	return nil
}

func ensureLogFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("创建日志文件失败: %w", err)
	}
	return file.Close()
}

// logViewerCommand 按平台返回打开文件的命令；Windows 使用 notepad，
// 避免 .log 未关联默认程序时弹出“打开方式”对话框。
func logViewerCommand(goos, path string) *exec.Cmd {
	switch goos {
	case "windows":
		return exec.Command("notepad.exe", path)
	case "darwin":
		return exec.Command("open", path)
	default:
		return exec.Command("xdg-open", path)
	}
}

// logFollowCommand 返回前台跟踪日志的命令，服务启动后交给终端继续显示运行日志。
func logFollowCommand(goos string) *exec.Cmd {
	const path = "./logs/client.log"
	if goos == "windows" {
		// Windows PowerShell 5 默认按系统代码页写控制台；显式使用 936，
		// 将 UTF-8 日志正确转换后输出到中文控制台，避免中文显示为乱码。
		command := "$OutputEncoding = [Text.Encoding]::GetEncoding(936); [Console]::OutputEncoding = $OutputEncoding; Get-Content -LiteralPath './logs/client.log' -Encoding UTF8 -Tail 50 -Wait"
		return exec.Command("powershell.exe", "-NoProfile", "-Command", command)
	}
	return exec.Command("tail", "-f", "-n", "50", path)
}
