package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/app_provider/apache"
	"github.com/certd/certd-client/internal/app_provider/iis"
	"github.com/certd/certd-client/internal/app_provider/nginx"
	"github.com/certd/certd-client/internal/elevation"
	"github.com/certd/certd-client/internal/logging"
	"github.com/certd/certd-client/internal/store"
	storeRepo "github.com/certd/certd-client/internal/store/repo"
	"github.com/certd/certd-client/internal/syncservice"
	"github.com/certd/certd-client/internal/tui"
	"github.com/certd/certd-client/internal/version"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/robfig/cron/v3"
)

func main() {
	defer func() {
		if recovered := recover(); recovered != nil {
			reportStartupError(fmt.Sprintf("程序崩溃：%v\n%s", recovered, debug.Stack()))
			os.Exit(1)
		}
	}()
	if isVersionCommand(os.Args[1:]) {
		fmt.Println(versionMessage())
		return
	}
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
	if err := run(os.Args[1:]); err != nil {
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

func run(args []string) error {
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
	providers := registeredProviders(runtime.GOOS)
	service := syncservice.New(siteRepo, providers, nil)
	if len(args) > 0 && args[0] != "tui" {
		switch strings.ToLower(args[0]) {
		case "sync":
			output := newConsoleAndLogOutput(logger, func(values ...any) { fmt.Println(values...) })
			result := service.RunConfigured(context.Background(), repo, settingsRepo, func(message string) { output(message) })
			writeSyncSummary(output, result)
			return syncResultError(result)
		case "start":
			schedule, expression, err := parseStartSchedule(args[1:], time.Now())
			if err != nil {
				return err
			}
			return runStart(schedule, expression, service, repo, settingsRepo, logger)
		case "version":
			fmt.Println(versionMessage())
			return nil
		default:
			return fmt.Errorf("未知命令 %q，可用命令：tui、sync、start、version", args[0])
		}
	}
	p := tea.NewProgram(tui.NewModelWithSettings(repo, siteRepo, settingsRepo, logger, providers), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	if requested, ok := finalModel.(interface{ StartRequested() bool }); ok && requested.StartRequested() {
		schedule, expression, err := parseStartSchedule(nil, time.Now())
		if err != nil {
			return err
		}
		return runStart(schedule, expression, service, repo, settingsRepo, logger)
	}
	return nil
}

func isVersionCommand(args []string) bool {
	return len(args) == 1 && strings.EqualFold(args[0], "version")
}

func versionMessage() string {
	return "certd-client " + version.String()
}

func parseStartSchedule(args []string, now time.Time) (cron.Schedule, string, error) {
	flags := flag.NewFlagSet("start", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	value := flags.String("cron", "", "Cron 表达式，例如 '30 2 * * *'")
	if err := flags.Parse(args); err != nil {
		return nil, "", err
	}
	if flags.NArg() > 0 {
		return nil, "", fmt.Errorf("不支持的 start 参数：%s", strings.Join(flags.Args(), " "))
	}
	expression := strings.TrimSpace(*value)
	if expression == "" {
		expression = fmt.Sprintf("%d %d * * *", now.Minute(), now.Hour())
	}
	schedule, err := cron.ParseStandard(expression)
	if err != nil {
		return nil, "", fmt.Errorf("解析 Cron 表达式失败：%w", err)
	}
	return schedule, expression, nil
}

func runStart(schedule cron.Schedule, expression string, service *syncservice.Service, apps *storeRepo.TargetAppRepository, settings *storeRepo.SettingsRepository, logger interface{ Println(...any) }) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	output := newConsoleAndLogOutput(logger, func(values ...any) { fmt.Println(values...) })
	run := func() {
		result := service.RunConfigured(ctx, apps, settings, func(message string) { output(message) })
		writeSyncSummary(output, result)
	}
	output(startSuccessMessage(expression))
	run()
	if ctx.Err() != nil {
		output("定时同步已停止")
		return nil
	}
	next := schedule.Next(time.Now())
	if next.IsZero() {
		return fmt.Errorf("Cron 表达式没有下一次执行时间：%s", expression)
	}
	output(nextExecutionMessage(next))
	for {
		wait := time.Until(next)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			output("定时同步已停止")
			return nil
		case <-timer.C:
			run()
			if ctx.Err() != nil {
				output("定时同步已停止")
				return nil
			}
			next = schedule.Next(time.Now())
			if next.IsZero() {
				return fmt.Errorf("Cron 表达式没有下一次执行时间：%s", expression)
			}
			output(nextExecutionMessage(next))
		}
	}
}

func startSuccessMessage(expression string) string {
	return "定时任务启动成功：" + expression
}

func nextExecutionMessage(next time.Time) string {
	return "下次执行时间：" + next.Format(time.DateTime)
}

func writeSyncSummary(output func(...any), result syncservice.Result) {
	output("============ 执行总结 =============")
	output(syncSummaryMessage(result.Succeeded, result.Skipped, len(result.Errors)))
	for _, item := range result.Errors {
		output("证书同步失败：" + item)
	}
}

func newConsoleAndLogOutput(logger interface{ Println(...any) }, console func(...any)) func(...any) {
	return func(values ...any) {
		if logger != nil {
			logger.Println(values...)
		}
		if console != nil {
			console(values...)
		}
	}
}

func syncSummaryMessage(succeeded, skipped, failed int) string {
	return fmt.Sprintf("执行总结：成功 %d，跳过 %d，失败 %d", succeeded, skipped, failed)
}

func syncResultError(result syncservice.Result) error {
	if result.Canceled || len(result.Errors) == 0 {
		return nil
	}
	return fmt.Errorf("证书同步失败 %d 项：%s", len(result.Errors), strings.Join(result.Errors, "；"))
}

func registeredProviders(goos string) *app_provider.Registry {
	providers := []app_provider.Provider{nginx.New(), apache.New()}
	if goos == "windows" {
		providers = append(providers, iis.New())
	}
	return app_provider.NewRegistry(providers...)
}
