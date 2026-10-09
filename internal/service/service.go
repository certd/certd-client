// Package service 将证书同步客户端封装为系统服务，
// 在 Windows 注册为服务、在 Linux 生成 systemd 单元，实现开机自启并按计划执行同步。
package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/certd/certd-client/internal/clientreport"
	"github.com/certd/certd-client/internal/logging"
	"github.com/certd/certd-client/internal/providers"
	"github.com/certd/certd-client/internal/schedule"
	"github.com/certd/certd-client/internal/store"
	storeRepo "github.com/certd/certd-client/internal/store/repo"
	"github.com/certd/certd-client/internal/syncservice"
	"github.com/certd/certd-client/internal/version"
	kardianos "github.com/kardianos/service"
)

const (
	// Name 是系统服务的内部名称。
	Name = "certd-client"
	// DisplayName 是服务在管理器中显示的名称。
	DisplayName = "Certd 证书同步客户端"
	// Description 是服务说明。
	Description = "Certd 官方证书部署客户端，按计划自动扫描站点并同步、部署证书。"
)

// runArgs 是服务进程被系统服务控制器启动时携带的参数，用于进入服务运行循环，
// 同时保证用户直接双击（无参数）时仍进入交互式 TUI。
var runArgs = []string{"service", "run"}

// Usage 返回 service 子命令的使用说明。
func Usage() string {
	return "用法：certd-client service <install|uninstall|start|stop|ensure|run>\n" +
		"  install   安装系统服务（Windows 需管理员，Linux 需 root）\n" +
		"  uninstall 卸载系统服务\n" +
		"  start     启动系统服务\n" +
		"  stop      停止系统服务\n" +
		"  ensure    确保系统服务已安装并处于运行状态（已注册则仅启动，供提权调用）\n" +
		"  run       由服务控制器调用，前台或后台运行定时同步"
}

// Classify 判断参数是否为 service 子命令，返回次级动作与是否命中。
// 缺少动作或未知动作统一返回 "help"。
func Classify(args []string) (string, bool) {
	if len(args) == 0 || !strings.EqualFold(args[0], "service") {
		return "", false
	}
	if len(args) < 2 {
		return "help", true
	}
	switch strings.ToLower(args[1]) {
	case "install", "uninstall", "start", "stop", "run", "ensure":
		return strings.ToLower(args[1]), true
	default:
		return "help", true
	}
}

// buildConfig 组装系统服务配置，工作目录固定为可执行文件所在目录，
// 保证 ./data 与 ./logs 落在安装目录。
func buildConfig(execPath, execDir string) *kardianos.Config {
	return &kardianos.Config{
		Name:             Name,
		DisplayName:      DisplayName,
		Description:      Description,
		WorkingDirectory: execDir,
		Arguments:        append([]string(nil), runArgs...),
		Executable:       serviceExecutablePath(execPath),
	}
}

func serviceExecutablePath(execPath string) string {
	ext := filepath.Ext(execPath)
	return filepath.Join(filepath.Dir(execPath), ".certd-client-service"+ext)
}

func syncServiceExecutable(execPath string) error {
	source, err := os.Open(execPath)
	if err != nil {
		return err
	}
	defer source.Close()
	targetPath := serviceExecutablePath(execPath)
	tmp, err := os.CreateTemp(filepath.Dir(targetPath), ".certd-client-service-*")
	if err != nil {
		return fmt.Errorf("创建服务副本失败：%w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = tmp.Close(); _ = os.Remove(tmpPath) }
	if _, err = io.Copy(tmp, source); err != nil {
		cleanup()
		return fmt.Errorf("复制服务副本失败：%w", err)
	}
	if err = tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("关闭服务副本失败：%w", err)
	}
	if err = os.Chmod(tmpPath, 0o755); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	for attempt := 0; attempt < 10; attempt++ {
		if err = os.Rename(tmpPath, targetPath); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = os.Remove(tmpPath)
	return fmt.Errorf("替换服务副本失败：%w", err)
}

func registeredExecutableMatches(expected string) bool {
	expected, err := filepath.Abs(expected)
	if err != nil {
		return false
	}
	var output []byte
	if runtime.GOOS == "linux" {
		output, err = exec.Command("systemctl", "show", Name+".service", "--property=ExecStart", "--value").Output()
	} else if runtime.GOOS == "windows" {
		output, err = exec.Command("sc.exe", "qc", Name).Output()
	} else if runtime.GOOS == "darwin" {
		return true
	} else {
		return false
	}
	if err != nil {
		return false
	}
	return executableInServiceOutput(string(output), expected)
}

func executableVersionMatches(executable string) bool {
	output, err := exec.Command(executable, "version").CombinedOutput()
	if err != nil {
		return false
	}
	fields := strings.Fields(string(output))
	for i, field := range fields {
		if field == "certd-client" && i+1 < len(fields) {
			return strings.TrimSpace(fields[i+1]) == version.String()
		}
	}
	return false
}

func executableInServiceOutput(output, expected string) bool {
	return strings.Contains(strings.ToLower(output), strings.ToLower(expected))
}

// handler 实现 kardianos 的服务接口，在 Start 中读取数据库里的定时计划并循环执行同步。
type handler struct {
	execDir string

	mu     sync.Mutex
	cancel context.CancelFunc
}

func (h *handler) Start(kardianos.Service) error {
	if h.execDir != "" {
		// 服务进程的工作目录可能不是安装目录，显式切换以保证相对路径一致。
		if err := os.Chdir(h.execDir); err != nil {
			return fmt.Errorf("切换工作目录失败：%w", err)
		}
	}
	db, err := store.OpenDatabase(filepath.Join("data", "certd-client.db"))
	if err != nil {
		return fmt.Errorf("打开数据库失败：%w", err)
	}
	logger := logging.Default()
	if logger == nil {
		return fmt.Errorf("打开日志失败：默认 logger 初始化失败")
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.mu.Lock()
	h.cancel = cancel
	h.mu.Unlock()

	apps := storeRepo.NewTargetAppRepository(db)
	sites := storeRepo.NewAppSiteRepository(db)
	settings := storeRepo.NewSettingsRepository(db)
	syncService := syncservice.New(sites, providers.Registered(runtime.GOOS), nil)
	reporter := clientreport.New(settings, apps, logger)
	go reporter.Run(ctx)

	output := func(message string) { logger.Info("%s", message) }
	output("Certd Client 当前版本：" + version.Version)
	output("Certd 证书同步服务已启动")

	value, err := settings.GetSetting(schedule.SettingKey)
	if err != nil {
		logger.Error("读取定时设置失败，按默认计划运行：%s", err.Error())
		value = ""
	}
	setting, err := schedule.Parse(value)
	if err != nil {
		logger.Error("解析定时设置失败，按默认计划运行：%s", err.Error())
		setting = schedule.Setting{Enabled: true}
	}
	if !setting.Enabled {
		output("定时同步已在设置中禁用，服务仅保持心跳在线")
		return nil
	}
	expression := schedule.ResolveExpression(setting.Cron, nowFunc())
	go func() {
		run := func() {
			result := syncService.RunConfigured(ctx, apps, settings, func(message string) { output(message) })
			logSyncSummary(logger, result)
		}
		if err := schedule.Run(ctx, expression, run, output); err != nil {
			logger.Error("定时同步循环异常退出：%s", err.Error())
		}
	}()
	return nil
}

func (h *handler) Stop(kardianos.Service) error {
	h.mu.Lock()
	cancel := h.cancel
	h.cancel = nil
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// nowFunc 获取当前时间，保留变量便于测试替换。
var nowFunc = time.Now

// logSyncSummary 将一轮同步结果写入日志，保持与 CLI 批处理总结一致的口径。
func logSyncSummary(logger *logging.Logger, result syncservice.Result) {
	logger.Info("%s", "============ 执行总结 =============")
	logger.Info("执行总结：成功 %d，跳过 %d，失败 %d", result.Succeeded, result.Skipped, len(result.Errors))
	for _, item := range result.Errors {
		logger.Info("%s", "证书同步失败："+item)
	}
}

// Run 依据 service 子命令分派安装、卸载、启停或服务运行循环。
func Run(args []string) error {
	action, ok := Classify(args)
	if !ok {
		return fmt.Errorf("不是 service 命令：%s", strings.Join(args, " "))
	}
	prg, err := newProgram()
	if err != nil {
		return err
	}
	switch action {
	case "install":
		execPath, err := os.Executable()
		if err != nil {
			return err
		}
		if err := syncServiceExecutable(execPath); err != nil {
			return err
		}
		return prg.Install()
	case "uninstall":
		return prg.Uninstall()
	case "start":
		return prg.Start()
	case "stop":
		return prg.Stop()
	case "ensure":
		execPath, err := os.Executable()
		if err != nil {
			return err
		}
		if err := syncServiceExecutable(execPath); err != nil {
			return err
		}
		return ensureRunning(prg)
	case "run":
		return prg.Run()
	default:
		fmt.Println(Usage())
		return nil
	}
}

// newProgram 依据当前可执行文件位置构造 kardianos 服务控制器。
func newProgram() (kardianos.Service, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("获取程序路径失败：%w", err)
	}
	execDir := filepath.Dir(execPath)
	cfg := buildConfig(execPath, execDir)
	prg, err := kardianos.New(&handler{execDir: execDir}, cfg)
	if err != nil {
		return nil, fmt.Errorf("初始化系统服务失败：%w", err)
	}
	return prg, nil
}

// serviceManager 抽象 ensureRunning 所需的最小能力，便于单元测试。
type serviceManager interface {
	Status() (kardianos.Status, error)
	Install() error
	Start() error
	Stop() error
}

// StopIfRunning 停止正在运行的系统服务。服务未安装时视为已停止，不返回错误。
func StopIfRunning() error {
	prg, err := newProgram()
	if err != nil {
		return err
	}
	status, err := prg.Status()
	if err != nil {
		if isServiceMissingError(err) {
			return nil
		}
		return fmt.Errorf("查询系统服务状态失败：%w", err)
	}
	if status != kardianos.StatusRunning {
		return nil
	}
	if err := prg.Stop(); err != nil {
		return fmt.Errorf("停止系统服务失败：%w", err)
	}
	return nil
}

// IsRunning 判断系统服务是否已安装且正在运行。未安装时返回 false、nil。
func IsRunning() (bool, error) {
	prg, err := newProgram()
	if err != nil {
		return false, err
	}
	status, err := prg.Status()
	if err != nil {
		if isServiceMissingError(err) {
			return false, nil
		}
		return false, err
	}
	return status == kardianos.StatusRunning, nil
}

// StartIfInstalled 启动已安装但未运行的系统服务。服务未安装时直接跳过。
func StartIfInstalled() error {
	prg, err := newProgram()
	if err != nil {
		return err
	}
	status, err := prg.Status()
	if err != nil {
		if isServiceMissingError(err) {
			return nil
		}
		return fmt.Errorf("查询系统服务状态失败：%w", err)
	}
	if status == kardianos.StatusRunning {
		return nil
	}
	if err := prg.Start(); err != nil {
		return fmt.Errorf("启动已安装系统服务失败：%w", err)
	}
	return nil
}

func isServiceMissingError(err error) bool {
	message := strings.ToLower(err.Error())
	for _, phrase := range []string{"not found", "does not exist", "不存在", "找不到", "no such service", "service unavailable", "service is not installed", "service not installed"} {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}

// EnsureRunning 确保系统服务已安装并处于运行状态：
// 已在运行则保持不变；已安装但停止则只启动，不重复安装；尚未安装则安装后启动。
func EnsureRunning() error {
	logging.Info("开始确保系统服务使用独立副本运行")
	var err error
	execPath, err := os.Executable()
	if err != nil {
		return err
	}
	execDir := filepath.Dir(execPath)
	prg, err := kardianos.New(&handler{execDir: execDir}, buildConfig(execPath, execDir))
	if err != nil {
		return fmt.Errorf("初始化系统服务失败：%w", err)
	}
	status, statusErr := prg.Status()
	installed := statusErr == nil
	logging.Info("系统服务状态检查完成：已安装=%t", installed)
	if statusErr != nil && !isServiceMissingError(statusErr) {
		return fmt.Errorf("查询系统服务状态失败：%w", statusErr)
	}
	if installed {
		servicePath := serviceExecutablePath(execPath)
		pathMatches := registeredExecutableMatches(servicePath)
		versionMatches := pathMatches && executableVersionMatches(servicePath)
		logging.Info("现有服务检查完成：路径一致=%t，版本一致=%t", pathMatches, versionMatches)
		if versionMatches {
			if status == kardianos.StatusRunning {
				return nil
			}
			logging.Info("系统服务副本版本一致，正在启动")
			if err := prg.Start(); err != nil {
				return fmt.Errorf("启动系统服务失败：%w", err)
			}
			return nil
		}
		logging.Info("正在停止现有系统服务")
		if err := prg.Stop(); err != nil && status == kardianos.StatusRunning {
			return fmt.Errorf("停止现有系统服务失败：%w", err)
		}
		logging.Info("系统服务路径或版本不一致，正在卸载旧服务")
		if err := prg.Uninstall(); err != nil {
			return fmt.Errorf("卸载旧系统服务失败：%w", err)
		}
		logging.Info("旧系统服务已卸载")
	}
	logging.Info("正在复制服务程序：%s", serviceExecutablePath(execPath))
	if err := syncServiceExecutable(execPath); err != nil {
		return err
	}
	if err := prg.Install(); err != nil {
		return fmt.Errorf("安装系统服务失败：%w", err)
	}
	logging.Info("系统服务注册完成，正在启动")
	if err := prg.Start(); err != nil {
		return fmt.Errorf("启动系统服务失败：%w", err)
	}
	logging.Info("系统服务启动完成")
	return nil
}

// Control 执行停止或卸载等服务控制动作。
func Control(action string) error {
	prg, err := newProgram()
	if err != nil {
		return err
	}
	switch action {
	case "stop":
		return prg.Stop()
	case "uninstall":
		if err := StopIfRunning(); err != nil {
			return err
		}
		return prg.Uninstall()
	default:
		return fmt.Errorf("不支持的服务操作：%s", action)
	}
}

// ControlService 提供给 TUI 的服务控制入口。
func ControlService(action string) error { return Control(action) }

// ensureRunning 依据服务当前状态决定安装与启动动作，避免对已注册服务重复安装。
func ensureRunning(prg serviceManager) error {
	status, err := prg.Status()
	switch {
	case err == nil && status == kardianos.StatusRunning:
		// 重新启动已运行服务，确保定时计划和新版本状态重新加载。
		if err := prg.Stop(); err != nil {
			return fmt.Errorf("停止已运行系统服务失败：%w", err)
		}
		if err := prg.Start(); err != nil {
			return fmt.Errorf("重新启动系统服务失败：%w", err)
		}
		return nil
	case err == nil && status == kardianos.StatusStopped:
		// 服务已注册但未运行，只启动，不重复注册。
		if err := prg.Start(); err != nil {
			return fmt.Errorf("启动系统服务失败：%w", err)
		}
		return nil
	default:
		// 状态无法确定，通常表示尚未安装：先安装再启动。
		if err := prg.Install(); err != nil {
			return fmt.Errorf("安装系统服务失败：%w", err)
		}
		if err := prg.Start(); err != nil {
			return fmt.Errorf("启动系统服务失败：%w", err)
		}
		return nil
	}
}
