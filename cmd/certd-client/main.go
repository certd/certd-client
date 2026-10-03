package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/certd/certd-client/internal/clientreport"
	"github.com/certd/certd-client/internal/elevation"
	"github.com/certd/certd-client/internal/logging"
	"github.com/certd/certd-client/internal/providers"
	"github.com/certd/certd-client/internal/schedule"
	systemservice "github.com/certd/certd-client/internal/service"
	"github.com/certd/certd-client/internal/store"
	storeRepo "github.com/certd/certd-client/internal/store/repo"
	"github.com/certd/certd-client/internal/syncservice"
	"github.com/certd/certd-client/internal/tui"
	"github.com/certd/certd-client/internal/updater"
	"github.com/certd/certd-client/internal/version"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// 最早配置崩溃输出，保证未捕获 panic 与 fatal error 也写入日志文件。
	// bubbletea 默认会吞掉 panic 只打印到 stdout（不写文件），所以这里另设兜底。
	configureCrashOutput()

	defer func() {
		if recovered := recover(); recovered != nil {
			reportStartupError(fmt.Sprintf("程序崩溃：%v\n%s", recovered, debug.Stack()))
			os.Exit(1)
		}
	}()
	if encoded, ok := updater.HelperArguments(os.Args[1:]); ok {
		if err := updater.RunHelper(encoded); err != nil {
			reportStartupError("更新失败：" + err.Error())
			os.Exit(1)
		}
		return
	}
	if isVersionCommand(os.Args[1:]) {
		fmt.Println(versionMessage())
		return
	}
	// service 子命令：安装/卸载/启停在 Windows 需要管理员（经 UAC 重启），
	// 而 service run 由系统服务控制器以已提权身份启动，绝不能再次触发提权。
	if action, ok := systemservice.Classify(os.Args[1:]); ok {
		if action != "run" && runtime.GOOS == "windows" {
			relaunched, err := elevation.New().Request()
			if err != nil {
				reportStartupError("请求管理员权限失败：" + err.Error())
				os.Exit(1)
			}
			if relaunched {
				return
			}
		}
		if action == "help" {
			fmt.Println(systemservice.Usage())
			return
		}
		if err := systemservice.Run(os.Args[1:]); err != nil {
			reportStartupError("服务操作失败：" + err.Error())
			os.Exit(1)
		}
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

// configureCrashOutput 让未捕获 panic 和 fatal error 额外写入 logs/client.log。
// 覆盖范围包括后台 Cmd goroutine（例如 textinput 的剪贴板粘贴）和 runtime fatal error，
// 这些不在 main 的 recover 覆盖范围内。
func configureCrashOutput() {
	logFile, err := openLogFile(logging.DefaultDir, logging.DefaultFileName)
	if err != nil {
		return
	}
	// SetCrashOutput 会复制文件描述符，因此可以立即关闭原文件。
	_ = debug.SetCrashOutput(logFile, debug.CrashOptions{})
	_ = logFile.Close()
}

func openLogFile(logDir, name string) (*os.File, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(logDir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

func reportStartupError(message string) {
	fmt.Fprintln(os.Stderr, message)
	writeStartupError(logging.DefaultDir, message)
}

func writeStartupError(logDir, message string) {
	logger, closer, err := logging.New(logDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "写入启动错误日志失败："+err.Error())
		return
	}
	logger.Error("启动失败：%s", message)
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
	logger, closer, err := logging.New(logging.DefaultDir)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	defer closer.Close()

	repo := storeRepo.NewTargetAppRepository(db)
	siteRepo := storeRepo.NewAppSiteRepository(db)
	settingsRepo := storeRepo.NewSettingsRepository(db)
	registry := providers.Registered(runtime.GOOS)
	service := syncservice.New(siteRepo, registry, nil)
	reporter := clientreport.New(settingsRepo, repo, logger)
	if len(args) > 0 && args[0] != "tui" {
		switch strings.ToLower(args[0]) {
		case "sync":
			logger.SetConsole(os.Stdout)
			output := func(values ...any) { logger.Info("%s", fmt.Sprint(values...)) }
			result := service.RunConfigured(context.Background(), repo, settingsRepo, func(message string) { output(message) })
			writeSyncSummary(output, result)
			reporter.Report(context.Background())
			return syncResultError(result)
		case "start":
			cronFlag, err := parseStartCronFlag(args[1:])
			if err != nil {
				return err
			}
			expression, disabled, err := resolveStartExpression(cronFlag, settingsRepo, time.Now())
			if err != nil {
				return err
			}
			if disabled {
				logger.SetConsole(os.Stdout)
				logger.Info("%s", startDisabledMessage)
				return nil
			}
			return runStart(expression, service, repo, settingsRepo, logger, reporter)
		case "version":
			fmt.Println(versionMessage())
			return nil
		default:
			return fmt.Errorf("未知命令 %q，可用命令：tui、sync、start、version", args[0])
		}
	}
	// TUI 运行期间同样周期上报心跳，保证打开界面时也算在线。
	heartbeatCtx, heartbeatCancel := context.WithCancel(context.Background())
	// WithoutCatchPanics 让 panic 传播到 main 的 recover，从而写入日志文件；
	// 否则 bubbletea 会吞掉 panic 只打印到 stdout，logs/client.log 留不下崩溃记录。
	tuiModel := tui.NewModelWithSettings(repo, siteRepo, settingsRepo, logger, registry)
	tuiModel.SetHeartbeatReporter(reporter)
	tuiModel.SetServiceStarter(func() error { return startBackgroundService(settingsRepo, time.Now) })
	tuiModel.SetServiceElevator(func() (*exec.Cmd, error) { return prepareTimedServiceElevation(settingsRepo, time.Now) })
	tuiModel.SetServiceAction(systemservice.ControlService)
	tuiModel.SetServiceActionElevator(prepareServiceActionElevation)
	p := tea.NewProgram(tuiModel, tea.WithAltScreen(), tea.WithoutCatchPanics())
	// p.Send 可能等待当前 Update 返回；异步投递避免 Update 内写日志时与自身死锁。
	logger.SetTUISink(func(message string) {
		go p.Send(tui.LogMessage(message))
	})
	go reporter.Run(heartbeatCtx)
	_, err = p.Run()
	heartbeatCancel()
	return err
}

// startBackgroundService 确保定时计划处于启用状态，然后将客户端注册并启动为系统服务。
// 服务已注册时仅确保启动，不会重复注册；适用于已具备权限的场景（Windows 已经 UAC 提权、Linux 已 root）。
func startBackgroundService(settings *storeRepo.SettingsRepository, now func() time.Time) error {
	if err := enableTimedSchedule(settings, now); err != nil {
		return err
	}
	return systemservice.EnsureRunning()
}

// enableTimedSchedule 读取并启用定时计划（补齐合法 Cron）后保存回设置表。
// 仅写用户自己的数据库，不需要提权，由父进程以当前用户身份完成。
func enableTimedSchedule(settings *storeRepo.SettingsRepository, now func() time.Time) error {
	value := ""
	if settings != nil {
		read, err := settings.GetSetting(schedule.SettingKey)
		if err != nil {
			return fmt.Errorf("读取定时设置失败：%w", err)
		}
		value = read
	}
	setting, err := schedule.Parse(value)
	if err != nil {
		setting = schedule.Setting{}
	}
	setting.Cron = schedule.ResolveExpression(setting.Cron, now())
	setting.Enabled = true
	content, err := setting.Marshal()
	if err != nil {
		return err
	}
	if settings != nil {
		if err := settings.SaveSetting(schedule.SettingKey, content); err != nil {
			return fmt.Errorf("保存定时设置失败：%w", err)
		}
	}
	return nil
}

// timedServiceElevationNeeded 判断是否需要通过 sudo 提权安装系统服务：
// 仅 Linux 且当前非 root 时需要（Windows 启动已经 UAC 提权，Linux 已 root 可直接安装）。
func timedServiceElevationNeeded(goos string, isRoot bool) bool {
	return goos == "linux" && !isRoot
}

// serviceElevationArgs 返回以 sudo 提权执行系统服务安装与启动的命令行参数。
func serviceElevationArgs(execPath string) []string {
	return []string{execPath, "service", "ensure"}
}

// prepareTimedServiceElevation 在需要提权时先以当前用户启用定时计划，再返回待以 sudo 执行的命令；
// 不需要提权时返回 (nil, nil)，由界面回退到普通启动流程。
func prepareTimedServiceElevation(settings *storeRepo.SettingsRepository, now func() time.Time) (*exec.Cmd, error) {
	if !timedServiceElevationNeeded(runtime.GOOS, os.Geteuid() == 0) {
		return nil, nil
	}
	if err := enableTimedSchedule(settings, now); err != nil {
		return nil, err
	}
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("获取程序路径失败：%w", err)
	}
	return exec.Command("sudo", serviceElevationArgs(execPath)...), nil
}

func prepareServiceActionElevation(action string) (*exec.Cmd, error) {
	if !timedServiceElevationNeeded(runtime.GOOS, os.Geteuid() == 0) {
		return nil, nil
	}
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("获取程序路径失败：%w", err)
	}
	return exec.Command("sudo", execPath, "service", action), nil
}

func isVersionCommand(args []string) bool {
	return len(args) == 1 && strings.EqualFold(args[0], "version")
}

func versionMessage() string {
	return "certd-client " + version.String()
}

// startDisabledMessage 用于未显式传入 --cron 且定时设置被禁用时的提示。
const startDisabledMessage = "定时同步已在设置中禁用，可在“定时设置”中启用，或使用 certd-client start --cron \"分 时 日 月 周\" 强制运行"

// parseStartCronFlag 解析 start 命令的 --cron 参数，未提供时返回空串。
func parseStartCronFlag(args []string) (string, error) {
	flags := flag.NewFlagSet("start", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	value := flags.String("cron", "", "Cron 表达式，例如 '30 2 * * *'")
	if err := flags.Parse(args); err != nil {
		return "", err
	}
	if flags.NArg() > 0 {
		return "", fmt.Errorf("不支持的 start 参数：%s", strings.Join(flags.Args(), " "))
	}
	return strings.TrimSpace(*value), nil
}

// resolveStartExpression 优先使用显式 --cron；否则读取数据库定时设置。
// 返回的 disabled 为 true 表示未显式传参且设置中已禁用定时同步。
func resolveStartExpression(cronFlag string, settings *storeRepo.SettingsRepository, now time.Time) (string, bool, error) {
	if cronFlag != "" {
		return cronFlag, false, nil
	}
	value := ""
	if settings != nil {
		read, err := settings.GetSetting(schedule.SettingKey)
		if err != nil {
			return "", false, fmt.Errorf("读取定时设置失败：%w", err)
		}
		value = read
	}
	setting, err := schedule.Parse(value)
	if err != nil {
		return "", false, err
	}
	if !setting.Enabled {
		return "", true, nil
	}
	return schedule.ResolveExpression(setting.Cron, now), false, nil
}

func runStart(expression string, service *syncservice.Service, apps *storeRepo.TargetAppRepository, settings *storeRepo.SettingsRepository, logger *logging.Logger, reporter *clientreport.Reporter) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.SetConsole(os.Stdout)
	output := func(values ...any) { logger.Info("%s", fmt.Sprint(values...)) }
	go reporter.Run(ctx)
	run := func() {
		result := service.RunConfigured(ctx, apps, settings, func(message string) { output(message) })
		writeSyncSummary(output, result)
	}
	return schedule.Run(ctx, expression, run, func(message string) { output(message) })
}

func writeSyncSummary(output func(...any), result syncservice.Result) {
	output("============ 执行总结 =============")
	output(syncSummaryMessage(result.Succeeded, result.Skipped, len(result.Errors)))
	for _, item := range result.Errors {
		output("证书同步失败：" + item)
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
