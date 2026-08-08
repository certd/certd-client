package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/app_provider/apache"
	"github.com/certd/certd-client/internal/app_provider/iis"
	"github.com/certd/certd-client/internal/app_provider/nginx"
	"github.com/certd/certd-client/internal/elevation"
	"github.com/certd/certd-client/internal/logging"
	"github.com/certd/certd-client/internal/store"
	"github.com/certd/certd-client/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if runtime.GOOS == "windows" {
		relaunched, err := elevation.New().Request()
		if err != nil {
			fmt.Fprintln(os.Stderr, "请求管理员权限失败："+err.Error())
			os.Exit(1)
		}
		if relaunched {
			return
		}
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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

	repo := store.NewTargetAppRepository(db)
	providers := app_provider.NewRegistry(nginx.New(), apache.New(), iis.New())
	p := tea.NewProgram(tui.NewModel(repo, logger, providers), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
