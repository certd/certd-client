package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/app_provider/apache"
	"github.com/certd/certd-client/internal/app_provider/iis"
	"github.com/certd/certd-client/internal/app_provider/nginx"
	"github.com/certd/certd-client/internal/elevation"
	"github.com/certd/certd-client/internal/logging"
	"github.com/certd/certd-client/internal/store"
	storeRepo "github.com/certd/certd-client/internal/store/repo"
	"github.com/certd/certd-client/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	defer func() {
		if recovered := recover(); recovered != nil {
			reportStartupError(fmt.Sprintf("程序崩溃：%v\n%s", recovered, debug.Stack()))
			os.Exit(1)
		}
	}()
	if runtime.GOOS == "windows" {
		relaunched, err := elevation.New().Request()
		if err != nil {
			reportStartupError("请求管理员权限失败：" + err.Error())
			os.Exit(1)
		}
		if relaunched {
			return
		}
	}
	if err := run(); err != nil {
		reportStartupError(err.Error())
		os.Exit(1)
	}
}

func reportStartupError(message string) {
	fmt.Fprintln(os.Stderr, message)
	writeStartupError("logs", message)
}

func writeStartupError(logDir, message string) {
	logger, closer, err := logging.New(logDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "写入启动错误日志失败："+err.Error())
		return
	}
	logger.Println("启动失败：" + message)
	if err := closer.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "关闭启动错误日志失败："+err.Error())
	}
}

func run() error {
	if err := os.MkdirAll("data", 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	db, err := store.OpenDatabase("data/certd-client.db")
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	logger, closer, err := logging.New("logs")
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	defer closer.Close()

	repo := storeRepo.NewTargetAppRepository(db)
	siteRepo := storeRepo.NewAppSiteRepository(db)
	settingsRepo := storeRepo.NewSettingsRepository(db)
	providers := app_provider.NewRegistry(nginx.New(), apache.New(), iis.New())
	p := tea.NewProgram(tui.NewModelWithSettings(repo, siteRepo, settingsRepo, logger, providers), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
