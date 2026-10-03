package tui

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openServicesConsole 打开系统服务管理面板，供用户将客户端设为开机自启。
// Windows 通过通用 shell 链接直接打开 services.msc；其他平台没有图形服务面板，
// 返回错误由界面提示对应的命令行操作。
func openServicesConsole() error {
	command := servicesCommand(runtime.GOOS)
	if command == nil {
		return fmt.Errorf("当前系统请使用 systemctl 管理开机自启服务")
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("打开系统服务管理失败: %w", err)
	}
	// 服务面板应在客户端退出后继续可用，因此不等待其结束，只回收子进程句柄。
	go func() { _ = command.Wait() }()
	return nil
}

// servicesCommand 返回打开服务管理面板的命令；仅 Windows 提供 services.msc 图形面板，
// 通过 `cmd /c start "" services.msc` 走系统通用链接直接打开，无需依赖具体安装路径。
func servicesCommand(goos string) *exec.Cmd {
	if goos == "windows" {
		return exec.Command("cmd", "/c", "start", "", "services.msc")
	}
	return nil
}

// servicesConsoleHint 返回非 Windows 平台管理服务、设置开机自启的命令提示。
func servicesConsoleHint(goos string) string {
	if goos == "windows" {
		return "正在打开系统服务管理（services.msc）"
	}
	return "请使用 systemctl 管理开机自启服务，例如：sudo systemctl enable certd-client && sudo systemctl start certd-client"
}
