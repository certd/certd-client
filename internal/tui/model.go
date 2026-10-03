package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/logging"
	storeRepo "github.com/certd/certd-client/internal/store/repo"
	"github.com/certd/certd-client/internal/updater"
	"github.com/certd/certd-client/internal/version"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screen uint8

const (
	homeScreen screen = iota
	rootInputScreen
	selectionScreen
	appManagementScreen
	siteListScreen
	deleteAppConfirmScreen
	certdSettingsScreen
	scheduleSettingsScreen
	serviceConfirmScreen
	scheduledActionsScreen
)

const logPageSize = 10

const (
	applicationIDWidth     = 6
	applicationTypeWidth   = 12
	applicationSiteWidth   = 8
	applicationHTTPSWidth  = 11
	applicationSyncedWidth = 6
	applicationFailedWidth = 4
	applicationStatusWidth = 5
)

type Model struct {
	repo                  *storeRepo.TargetAppRepository
	siteRepo              *storeRepo.AppSiteRepository
	settingsRepo          *storeRepo.SettingsRepository
	providers             *app_provider.Registry
	logger                logging.Log
	heartbeat             HeartbeatReporter
	menuCursor            int
	screen                screen
	rootInput             textinput.Model
	discovered            []app_provider.App
	selected              map[string]bool
	selectCursor          int
	apps                  []storeRepo.TargetApp
	appManageCursor       int
	appManageOffset       int
	managedApp            storeRepo.TargetApp
	managedSites          []storeRepo.AppSite
	siteManageCursor      int
	siteManageOffset      int
	logs                  []string
	logScroll             int
	scanning              bool
	siteScanning          bool
	scanProgress          app_provider.Progress
	scanProgressCh        chan app_provider.Progress
	status                string
	certdInputs           [5]textinput.Model
	certdFocus            int
	scheduleInput         textinput.Model
	scheduleEnabled       bool
	startService          func() error
	serviceAction         func(string) error
	scheduleReturnScreen  screen
	serviceElevator       func() (*exec.Cmd, error)
	serviceActionElevator func(string) (*exec.Cmd, error)
	syncing               bool
	syncProgressCh        chan string
	syncCancel            context.CancelFunc
	startRequested        bool
	openLog               func(path string) error
	openServices          func() error
	updateResult          *updater.Result
	updateChecking        bool
	width, height         int
}

// HeartbeatReporter 是接口设置保存后立即上报心跳所需的最小能力。
type HeartbeatReporter interface {
	Report(context.Context)
}

// SetHeartbeatReporter 设置接口配置保存后使用的心跳上报器。
func (m *Model) SetHeartbeatReporter(reporter HeartbeatReporter) {
	m.heartbeat = reporter
}

// SetServiceStarter 设置“定时同步”确认后用于注册并启动后台系统服务的启动器。
func (m *Model) SetServiceStarter(starter func() error) {
	m.startService = starter
}

// SetServiceAction 设置停止和卸载系统服务的执行器。
func (m *Model) SetServiceAction(action func(string) error) { m.serviceAction = action }

// SetServiceElevator 设置 Linux 非 root 下以 sudo 提权安装系统服务的准备器。
// 返回非 nil 命令表示需要提权（已备好待以 sudo 执行的命令）；
// 返回 (nil, nil) 表示无需提权，界面回退到普通启动流程。
func (m *Model) SetServiceElevator(elevator func() (*exec.Cmd, error)) {
	m.serviceElevator = elevator
}

// SetServiceActionElevator 设置 Linux 非 root 下执行停止或卸载服务的 sudo 命令准备器。
func (m *Model) SetServiceActionElevator(elevator func(string) (*exec.Cmd, error)) {
	m.serviceActionElevator = elevator
}

var menuItems = []string{"应用扫描", "站点扫描", "应用管理", "Certd接口设置", "同步证书", "定时执行", "打开日志"}

// 菜单下标常量：新增菜单项时需同步调整，并保证与 menuItems 及 menuHelp 一致。
const (
	scheduledSyncMenuIndex = 5 // “定时同步”
	openLogMenuIndex       = 6 // “打开日志”，不受耗时任务限制
	updateMenuIndex        = 7 // “更新版本”，由 View 动态追加
)

var scheduledActionItems = []string{"启动定时同步服务", "定时设置", "停止服务", "卸载服务", "服务管理"}

func NewModel(repo *storeRepo.TargetAppRepository, siteRepo *storeRepo.AppSiteRepository, logger logging.Log, registry *app_provider.Registry) Model {
	return NewModelWithSettings(repo, siteRepo, nil, logger, registry)
}

func NewModelWithSettings(repo *storeRepo.TargetAppRepository, siteRepo *storeRepo.AppSiteRepository, settingsRepo *storeRepo.SettingsRepository, logger logging.Log, registry *app_provider.Registry) Model {
	input := textinput.New()
	input.Placeholder = "例如 /etc 或 C:\\Web"
	input.CharLimit = 2048
	input.Width = 60
	providers := registry
	certdInputs := [5]textinput.Model{}
	for i, placeholder := range []string{"https://certd.example.com", "keyId", "keySecret", "本机名称（可选）", "10"} {
		certdInputs[i] = textinput.New()
		certdInputs[i].Placeholder = placeholder
		certdInputs[i].CharLimit = 2048
		certdInputs[i].Width = 60
	}
	certdInputs[2].EchoMode = textinput.EchoPassword
	scheduleInput := textinput.New()
	scheduleInput.Placeholder = "分 时 日 月 周，例如 30 2 * * *"
	scheduleInput.CharLimit = 128
	scheduleInput.Width = 40
	return Model{
		repo: repo, siteRepo: siteRepo, settingsRepo: settingsRepo, providers: providers, logger: logger,
		rootInput: input, certdInputs: certdInputs, scheduleInput: scheduleInput, scheduleEnabled: true,
		selected: make(map[string]bool),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg { return appsLoadedMsg{apps: m.loadApps()} }, m.checkUpdates())
}

// StartRequested reports whether the user selected the TUI entry that switches to CLI start mode.
func (m Model) StartRequested() bool {
	return m.startRequested
}

type appsLoadedMsg struct {
	apps []storeRepo.TargetApp
}

type scanCompletedMsg struct {
	apps []app_provider.App
	err  error
}

type scanProgressTickMsg struct{}

type siteScanCompletedMsg struct {
	appCount      int
	siteCount     int
	disabledCount int
	httpsCount    int
	newCount      int
	errors        []string
}

type siteScanSummary struct {
	siteCount     int
	disabledCount int
	httpsCount    int
	newCount      int
}

type certificateSyncCompletedMsg struct {
	result certificateSyncResult
}

type syncProgressTickMsg struct{}

type logOpenedMsg struct {
	path string
	err  error
}

type servicesOpenedMsg struct {
	err error
}

// serviceActionResultMsg 是后台注册并启动系统服务后回传的结果。
type serviceActionResultMsg struct {
	action string
	err    error
}

type updateCheckedMsg struct {
	result *updater.Result
	err    error
}

// LogMessage 是后台任务投递给 TUI 的简要日志消息。
type LogMessage string

// openLogCommand 在后台用系统默认程序打开日志文件，失败原因回传到界面并写入日志。
func (m Model) openLogCommand() tea.Cmd {
	path := logging.DefaultPath()
	open := m.openLog
	if open == nil {
		open = openLogFileWithSystemViewer
	}
	return func() tea.Msg {
		return logOpenedMsg{path: path, err: open(path)}
	}
}

// openServicesCommand 在后台打开系统服务管理面板（Windows services.msc），
// 失败原因回传到界面并写入日志。
func (m Model) openServicesCommand() tea.Cmd {
	open := m.openServices
	if open == nil {
		open = openServicesConsole
	}
	return func() tea.Msg {
		return servicesOpenedMsg{err: open()}
	}
}

// startServiceCommand 在后台执行“定时同步”确认后的服务注册与启动，
// 结果回传到界面；启动器未注入时返回明确错误。
func (m Model) startServiceCommand() tea.Cmd {
	start := m.startService
	if start == nil {
		return func() tea.Msg {
			return serviceActionResultMsg{action: "ensure", err: fmt.Errorf("后台服务启动器未初始化")}
		}
	}
	return func() tea.Msg {
		return serviceActionResultMsg{action: "ensure", err: start()}
	}
}

func (m Model) checkUpdates() tea.Cmd {
	return func() tea.Msg {
		m.logInfo("开始检查版本：AtomGit、GitHub")
		result, err := updater.Check(context.Background(), nil, runtime.GOOS, runtime.GOARCH)
		return updateCheckedMsg{result: &result, err: err}
	}
}

func (m Model) loadApps() []storeRepo.TargetApp {
	if m.repo == nil {
		return nil
	}
	apps, err := m.repo.List()
	if err != nil {
		return nil
	}
	return apps
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case LogMessage:
		m.appendDisplayLog(string(msg))
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case appsLoadedMsg:
		m.apps = msg.apps
	case updateCheckedMsg:
		m.updateChecking = false
		if msg.err != nil {
			m.status = "检查更新失败：" + msg.err.Error()
			m.logInfo("检查更新失败：" + msg.err.Error())
			return m, nil
		}
		m.updateResult = msg.result
		if msg.result == nil {
			m.status = "检查更新失败：未返回版本信息"
			m.logInfo(m.status)
			return m, nil
		}
		for _, channel := range msg.result.Channels {
			m.logInfo(fmt.Sprintf("版本检查结果：%s v%s，响应 %s", channel.Name, channel.Version, channel.Latency.Round(time.Millisecond)))
		}
		if msg.result != nil && msg.result.Version != version.String() {
			m.status = "发现新版本 v" + msg.result.Version
			m.logInfo("发现新版本：当前 v" + version.String() + "，最新 v" + msg.result.Version + "，最快渠道 " + msg.result.Fastest.Name)
		} else {
			m.status = "当前已是最新版本 v" + version.String()
		}
	case scanProgressTickMsg:
		if !m.scanning {
			return m, nil
		}
		m.readScanProgress()
		if m.scanProgress.ProviderType == "" {
			m.status = fmt.Sprintf("扫描中：已扫描 %d 个目录，剩余 %d 个目录", m.scanProgress.ScannedDirectories, m.scanProgress.RemainingDirectories)
		} else {
			m.status = fmt.Sprintf("扫描中（%s）：已扫描 %d 个目录，剩余 %d 个目录", m.scanProgress.ProviderType, m.scanProgress.ScannedDirectories, m.scanProgress.RemainingDirectories)
		}
		m.logInfo(m.status)
		return m, scanProgressTick()
	case scanCompletedMsg:
		m.scanning = false
		m.scanProgressCh = nil
		if msg.err != nil {
			m.status = "扫描失败：" + msg.err.Error()
			m.logInfo(m.status)
			return m, nil
		}
		m.discovered = msg.apps
		m.selected = make(map[string]bool, len(msg.apps))
		m.selectCursor = 0
		m.screen = selectionScreen
		m.status = fmt.Sprintf("扫描完成，发现 %d 个应用安装目录", len(msg.apps))
		m.logInfo(m.status)
	case siteScanCompletedMsg:
		m.siteScanning = false
		m.apps = m.loadApps()
		m.status = fmt.Sprintf("站点扫描完成：扫描 %d 个应用，发现 %d 个站点，禁用 %d 个，HTTPS %d 个，新增 %d 个", msg.appCount, msg.siteCount, msg.disabledCount, msg.httpsCount, msg.newCount)
		if len(msg.errors) > 0 {
			m.status += fmt.Sprintf("，失败 %d 个", len(msg.errors))
		}
		m.logInfo(m.status)
		for _, scanErr := range msg.errors {
			m.logInfo("站点扫描失败：" + scanErr)
		}
	case certificateSyncCompletedMsg:
		m.syncing = false
		m.syncCancel = nil
		m.readSyncProgress()
		m.syncProgressCh = nil
		m.apps = m.loadApps()
		if msg.result.Canceled {
			m.status = fmt.Sprintf("证书同步已取消：成功 %d，跳过 %d", msg.result.Succeeded, msg.result.Skipped)
		} else {
			m.status = fmt.Sprintf("证书同步完成：成功 %d，跳过 %d，失败 %d", msg.result.Succeeded, msg.result.Skipped, len(msg.result.Errors))
		}
		m.logInfo(m.status)
		for _, item := range msg.result.Errors {
			m.logInfo("证书同步失败：" + item)
		}
	case logOpenedMsg:
		if msg.err != nil {
			m.status = "打开日志文件失败：" + msg.err.Error()
		} else {
			m.status = "已打开日志文件：" + msg.path
		}
		m.logInfo(m.status)
	case servicesOpenedMsg:
		if msg.err != nil {
			m.status = "打开服务管理失败：" + msg.err.Error()
		} else {
			m.status = "已打开系统服务管理，可在其中将客户端设为开机自启"
		}
		m.logInfo(m.status)
	case serviceActionResultMsg:
		if msg.action == "" {
			m.status = "服务启动失败：未收到服务启动结果"
			m.logInfo(m.status)
			return m, nil
		}
		if msg.err != nil {
			m.status = serviceActionFailure(msg.action, msg.err.Error())
			m.logInfo(m.status)
			return m, nil
		}
		m.status = serviceActionSuccess(msg.action)
		m.logInfo(m.status)
		if msg.action == "ensure" {
			m.screen = homeScreen
			return m, nil
		}
		m.screen = scheduledActionsScreen
		return m, nil
	case syncProgressTickMsg:
		if !m.syncing {
			return m, nil
		}
		m.readSyncProgress()
		return m, syncProgressTick()
	case tea.KeyMsg:
		if msg.String() == "esc" && m.syncing {
			if m.syncCancel != nil {
				m.syncCancel()
			}
			m.status = "正在取消证书同步"
			m.logInfo(m.status)
			return m, nil
		}
		if msg.String() == "ctrl+c" || (msg.String() == "q" && m.screen == homeScreen) {
			return m, tea.Quit
		}
		switch msg.String() {
		case "pgup", "pageup":
			m.scrollLogs(1)
			return m, nil
		case "pgdown", "pagedown":
			m.scrollLogs(-1)
			return m, nil
		}
		switch m.screen {
		case homeScreen:
			return m.updateHome(msg)
		case rootInputScreen:
			return m.updateRootInput(msg)
		case selectionScreen:
			return m.updateSelection(msg)
		case appManagementScreen:
			return m.updateAppManagement(msg)
		case siteListScreen:
			return m.updateSiteList(msg)
		case deleteAppConfirmScreen:
			return m.updateDeleteAppConfirm(msg)
		case certdSettingsScreen:
			return m.updateCertdSettings(msg)
		case scheduleSettingsScreen:
			return m.updateScheduleSettings(msg)
		case serviceConfirmScreen:
			return m.updateServiceConfirm(msg)
		case scheduledActionsScreen:
			return m.updateScheduledActions(msg)
		}
	}
	return m, nil
}

func (m Model) updateHome(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k", "left":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
	case "down", "j", "right":
		if m.menuCursor < updateMenuIndex {
			m.menuCursor++
		}
	case "enter", " ":
		// 打开日志不受“已有任务正在执行”限制：同步失败时正是最需要查看日志的时刻。
		if m.menuCursor == openLogMenuIndex {
			m.status = "正在打开日志文件：" + logging.DefaultPath()
			m.logInfo(m.status)
			return m, m.openLogCommand()
		}
		if m.menuCursor == updateMenuIndex {
			if m.updateResult == nil || m.updateResult.Version == version.String() {
				m.status = "正在检查更新"
				m.updateChecking = true
				return m, m.checkUpdates()
			}
			// 检测到新版本只提示，不自动下载安装、替换或重启，避免影响正在运行的同步任务。
			m.status = "发现新版本 v" + m.updateResult.Version + "，已停用自动更新，请通过安装脚本或发布页手动更新"
			m.logInfo(m.status)
			return m, nil
		}
		if m.scanning || m.siteScanning || m.syncing {
			m.status = "已有任务正在执行"
			return m, nil
		}
		if m.menuCursor == 0 {
			m.screen = rootInputScreen
			m.rootInput.Reset()
			m.rootInput.Focus()
			m.status = "请输入扫描根目录，回车开始扫描"
			m.logInfo("开始应用扫描：等待输入根目录")
		} else if m.menuCursor == 1 {
			if m.scanning || m.siteScanning || m.syncing {
				m.status = "已有扫描任务正在执行"
				return m, nil
			}
			m.apps = m.loadApps()
			activeApps := make([]storeRepo.TargetApp, 0, len(m.apps))
			for _, app := range m.apps {
				if app.Enabled {
					activeApps = append(activeApps, app)
				}
			}
			if len(activeApps) == 0 {
				m.status = "暂无已启用应用，无法扫描站点"
				m.logInfo(m.status)
				return m, nil
			}
			m.siteScanning = true
			m.status = fmt.Sprintf("开始扫描 %d 个已登记应用的站点", len(activeApps))
			m.logInfo(m.status)
			return m, m.scanSites(activeApps)
		} else if m.menuCursor == 2 {
			m.apps = m.loadApps()
			m.appManageCursor = 0
			m.appManageOffset = 0
			m.managedSites = nil
			m.screen = appManagementScreen
			m.status = fmt.Sprintf("应用管理：当前已登记 %d 个应用", len(m.apps))
			m.logInfo(m.status)
		} else if m.menuCursor == 3 {
			m.loadCertdSettings()
			m.certdFocus = 0
			for i := range m.certdInputs {
				m.certdInputs[i].Blur()
			}
			m.certdInputs[0].Focus()
			m.screen = certdSettingsScreen
			m.status = "请输入 Certd 接口配置，回车保存，Esc 返回"
			m.logInfo("打开 Certd 接口设置")
		} else if m.menuCursor == 4 {
			if m.scanning || m.siteScanning || m.syncing {
				m.status = "已有任务正在执行"
				return m, nil
			}
			m.apps = m.loadApps()
			activeApps := make([]storeRepo.TargetApp, 0, len(m.apps))
			for _, app := range m.apps {
				if app.Enabled {
					activeApps = append(activeApps, app)
				}
			}
			if len(activeApps) == 0 {
				m.status = "暂无已启用应用，无法同步证书"
				m.logInfo(m.status)
				return m, nil
			}
			m.syncing = true
			m.syncProgressCh = make(chan string, 32)
			syncContext, cancel := context.WithCancel(context.Background())
			m.syncCancel = cancel
			m.status = fmt.Sprintf("开始同步 %d 个应用的证书", len(activeApps))
			m.logInfo(m.status)
			return m, tea.Batch(m.syncCertificatesService(syncContext, activeApps, m.syncProgressCh), syncProgressTick())
		} else if m.menuCursor == scheduledSyncMenuIndex {
			m.screen = scheduledActionsScreen
			m.selectCursor = 0
			m.status = "选择定时服务操作 · Esc 返回"
		}
	}
	return m, nil
}

func (m Model) updateScheduledActions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = homeScreen
		return m, nil
	case "up", "k", "left":
		if m.selectCursor > 0 {
			m.selectCursor--
		}
	case "down", "j", "right":
		if m.selectCursor < len(scheduledActionItems)-1 {
			m.selectCursor++
		}
	case "enter", " ":
		switch m.selectCursor {
		case 0:
			m.screen = serviceConfirmScreen
			m.status = "将注册并启动定时同步服务。按 y 确认，Esc 取消"
			m.logInfo("准备启动后台定时同步服务，等待确认")
		case 1:
			m.loadSchedule()
			m.scheduleInput.Focus()
			m.screen = scheduleSettingsScreen
			m.scheduleReturnScreen = scheduledActionsScreen
			m.status = "输入 Cron 表达式（分 时 日 月 周），回车保存，e 切换启用，Esc 返回"
			m.logInfo("打开定时设置")
		case 2, 3:
			action := "stop"
			if m.selectCursor == 3 {
				action = "uninstall"
			}
			if m.serviceActionElevator != nil {
				cmd, err := m.serviceActionElevator(action)
				if err != nil {
					m.status = serviceActionFailure(action, err.Error())
					m.logInfo(m.status)
					return m, nil
				}
				if cmd != nil {
					m.status = "正在通过 sudo 执行服务操作……"
					return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return serviceActionResultMsg{action: action, err: err} })
				}
			}
			return m, m.runServiceActionCommand(action)
		case 4:
			if runtime.GOOS == "windows" {
				m.status = "正在打开系统服务管理（services.msc）"
				m.logInfo(m.status)
				return m, m.openServicesCommand()
			}
			m.status = servicesConsoleHint(runtime.GOOS)
			m.logInfo(m.status)
		}
	}
	return m, nil
}

func (m Model) runServiceActionCommand(action string) tea.Cmd {
	return func() tea.Msg {
		if m.serviceAction == nil {
			return serviceActionResultMsg{action: action, err: fmt.Errorf("服务操作器未初始化")}
		}
		return serviceActionResultMsg{action: action, err: m.serviceAction(action)}
	}
}

// updateServiceConfirm 处理“定时同步”启动后台服务前的确认屏：
// y/回车 确认后在后台注册并启动服务；Esc/n 取消返回主菜单。
func (m Model) updateServiceConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "n":
		m.screen = homeScreen
		m.status = "已取消启动后台定时同步服务"
		m.logInfo(m.status)
	case "y", "enter":
		if m.scanning || m.siteScanning || m.syncing {
			m.status = "已有任务正在执行，请稍候再启动后台服务"
			return m, nil
		}
		// Linux 非 root 下安装 systemd 服务需要提权：交由 sudo 子进程执行，
		// 借 tea.ExecProcess 将终端让给 sudo，以便输入密码。
		if m.serviceElevator != nil {
			cmd, err := m.serviceElevator()
			if err != nil {
				m.status = "启动后台定时同步服务失败：" + err.Error()
				m.logInfo(m.status)
				return m, nil
			}
			if cmd != nil {
				m.status = "正在以 root 权限安装并启动系统服务，请在终端提示输入 sudo 密码……"
				m.logInfo(m.status)
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
					if err != nil {
						return serviceActionResultMsg{action: "ensure", err: fmt.Errorf("sudo 服务注册或启动失败：%w", err)}
					}
					return serviceActionResultMsg{action: "ensure"}
				})
			}
		}
		m.status = "正在注册并启动后台定时同步服务……"
		m.logInfo(m.status)
		return m, m.startServiceCommand()
	}
	return m, nil
}

func (m Model) updateAppManagement(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.apps) == 0 {
		if msg.String() == "esc" {
			m.screen = homeScreen
		}
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.screen = homeScreen
	case "up", "k":
		if m.appManageCursor > 0 {
			m.appManageCursor--
			m.appManageOffset = keepTableCursorVisible(m.appManageCursor, len(m.apps), m.height, m.appManageOffset)
		}
	case "down", "j":
		if m.appManageCursor < len(m.apps)-1 {
			m.appManageCursor++
			m.appManageOffset = keepTableCursorVisible(m.appManageCursor, len(m.apps), m.height, m.appManageOffset)
		}
	case "enter":
		if m.repo == nil {
			m.status = "数据库未初始化"
			m.logInfo(m.status)
			return m, nil
		}
		m.managedApp = m.apps[m.appManageCursor]
		if m.siteRepo == nil {
			m.status = "站点仓库未初始化"
			m.logInfo(m.status)
			return m, nil
		}
		sites, err := m.siteRepo.ListSites(m.managedApp.ID)
		if err != nil {
			m.status = "读取站点失败：" + err.Error()
			m.logInfo(m.status)
			return m, nil
		}
		m.managedSites = sites
		m.siteManageCursor = 0
		m.siteManageOffset = 0
		m.screen = siteListScreen
		m.status = fmt.Sprintf("查看应用站点：%s", m.managedApp.RootDir)
		m.logInfo(m.status)
	case "d", "delete":
		m.managedApp = m.apps[m.appManageCursor]
		m.screen = deleteAppConfirmScreen
		m.status = "等待确认删除应用"
	}
	return m, nil
}

func (m Model) updateSiteList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.screen = appManagementScreen
		m.status = "返回应用管理"
		return m, nil
	}
	if len(m.managedSites) == 0 {
		return m, nil
	}
	switch msg.String() {
	case "up", "k":
		if m.siteManageCursor > 0 {
			m.siteManageCursor--
			m.siteManageOffset = keepTableCursorVisible(m.siteManageCursor, len(m.managedSites), m.height, m.siteManageOffset)
		}
	case "down", "j":
		if m.siteManageCursor < len(m.managedSites)-1 {
			m.siteManageCursor++
			m.siteManageOffset = keepTableCursorVisible(m.siteManageCursor, len(m.managedSites), m.height, m.siteManageOffset)
		}
	case " ":
		if m.siteRepo == nil {
			m.status = "站点仓库未初始化"
			m.logInfo(m.status)
			return m, nil
		}
		site := &m.managedSites[m.siteManageCursor]
		enabled := !site.Enabled
		if err := m.siteRepo.SetEnabled(site.ID, enabled); err != nil {
			m.status = "更新站点状态失败：" + err.Error()
			m.logInfo(m.status)
			return m, nil
		}
		site.Enabled = enabled
		if enabled {
			m.status = "已启用站点：" + site.PrimaryDomain
		} else {
			m.status = "已禁用站点：" + site.PrimaryDomain
		}
		m.logInfo(m.status)
	}
	return m, nil
}

func (m Model) updateDeleteAppConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "n":
		m.screen = appManagementScreen
		m.status = "已取消删除应用"
	case "y":
		if m.repo == nil {
			m.status = "数据库未初始化"
			m.logInfo(m.status)
			return m, nil
		}
		deletedSites, err := m.repo.DeleteApp(m.managedApp.ID)
		if err != nil {
			m.status = "删除应用失败：" + err.Error()
			m.logInfo(m.status)
			return m, nil
		}
		m.apps = m.loadApps()
		if m.appManageCursor >= len(m.apps) && m.appManageCursor > 0 {
			m.appManageCursor--
		}
		m.managedSites = nil
		m.screen = appManagementScreen
		m.status = fmt.Sprintf("已删除应用及 %d 个站点", deletedSites)
		m.logInfo(m.status)
	}
	return m, nil
}

func (m Model) scanSites(apps []storeRepo.TargetApp) tea.Cmd {
	return func() tea.Msg {
		result := siteScanCompletedMsg{appCount: len(apps)}
		if m.repo == nil {
			result.errors = append(result.errors, "数据库未初始化")
			return result
		}
		for _, app := range apps {
			if m.providers == nil {
				result.errors = append(result.errors, "应用 Provider 未注册")
				continue
			}
			provider, ok := m.providers.Find(app.AppType)
			if !ok {
				result.errors = append(result.errors, fmt.Sprintf("%s：未找到 %s Provider", app.RootDir, app.AppType))
				continue
			}
			sites, err := provider.ScanSites(app_provider.App{RootDir: app.RootDir, AppType: app.AppType})
			if err != nil {
				result.errors = append(result.errors, fmt.Sprintf("%s：%v", app.RootDir, err))
				continue
			}
			records := make([]storeRepo.AppSite, 0, len(sites))
			for _, site := range sites {
				records = append(records, storeRepo.AppSite{
					PrimaryDomain:   site.PrimaryDomain,
					Domains:         strings.Join(site.Domains, ","),
					SubdomainCount:  site.SubdomainCount,
					ConfigPath:      site.ConfigPath,
					CertificatePath: site.CertificatePath,
					PrivateKeyPath:  site.PrivateKeyPath,
					DeploymentName:  site.DeploymentName,
					Https:           site.Https,
				})
			}
			if m.siteRepo == nil {
				result.errors = append(result.errors, "站点仓库未初始化")
				continue
			}
			existing, err := m.siteRepo.ListSites(app.ID)
			if err != nil {
				result.errors = append(result.errors, fmt.Sprintf("%s：读取旧站点失败：%v", app.RootDir, err))
				continue
			}
			summary := summarizeSiteScan(existing, records)
			if err := m.siteRepo.SyncSites(app.ID, records); err != nil {
				result.errors = append(result.errors, fmt.Sprintf("%s：%v", app.RootDir, err))
				continue
			}
			result.siteCount += summary.siteCount
			result.disabledCount += summary.disabledCount
			result.httpsCount += summary.httpsCount
			result.newCount += summary.newCount
		}
		return result
	}
}

func summarizeSiteScan(existing []storeRepo.AppSite, discovered []storeRepo.AppSite) siteScanSummary {
	known := make(map[string]bool, len(existing))
	for _, site := range existing {
		known[site.PrimaryDomain+"\x00"+site.ConfigPath] = site.Enabled
	}
	summary := siteScanSummary{siteCount: len(discovered)}
	for _, site := range discovered {
		if site.Https {
			summary.httpsCount++
		}
		enabled, found := known[site.PrimaryDomain+"\x00"+site.ConfigPath]
		if !found {
			summary.newCount++
		} else if !enabled {
			summary.disabledCount++
		}
	}
	return summary
}

func (m Model) updateRootInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.scanning {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.screen = homeScreen
		m.rootInput.Blur()
		return m, nil
	case "enter":
		root := strings.TrimSpace(m.rootInput.Value())
		if root == "" {
			m.status = "根目录不能为空"
			m.logInfo("扫描失败：根目录为空")
			return m, nil
		}
		if m.providers == nil || len(m.providers.All()) == 0 {
			m.status = "未注册应用扫描 Provider"
			m.logInfo(m.status)
			return m, nil
		}
		if m.repo != nil {
			disabledApps, err := m.repo.DisableMissingApps()
			if err != nil {
				m.status = "检查已登记应用失败：" + err.Error()
				m.logInfo(m.status)
				return m, nil
			}
			if len(disabledApps) > 0 {
				m.apps = m.loadApps()
				m.logInfo(fmt.Sprintf("已禁用 %d 个不存在的应用目录", len(disabledApps)))
			}
		}
		m.scanning = true
		m.scanProgress = app_provider.Progress{RemainingDirectories: 1}
		m.scanProgressCh = make(chan app_provider.Progress, 1)
		m.rootInput.Blur()
		m.status = "开始扫描，请稍候"
		m.logInfo("开始扫描根目录：" + root)
		return m, tea.Batch(m.scanApps(root, m.scanProgressCh), scanProgressTick())
	default:
		var cmd tea.Cmd
		m.rootInput, cmd = m.rootInput.Update(msg)
		return m, cmd
	}
}

func (m Model) scanApps(root string, progress chan app_provider.Progress) tea.Cmd {
	return func() tea.Msg {
		if m.providers == nil {
			return scanCompletedMsg{err: fmt.Errorf("应用扫描 Provider 未注册")}
		}
		apps := make([]app_provider.App, 0)
		for _, provider := range m.providers.All() {
			found, err := provider.ScanApps(root, newProgressPublisher(progress))
			if err != nil {
				return scanCompletedMsg{err: fmt.Errorf("扫描 %s 应用: %w", provider.Type(), err)}
			}
			apps = append(apps, found...)
		}
		return scanCompletedMsg{apps: apps}
	}
}

func newProgressPublisher(progress chan app_provider.Progress) func(app_provider.Progress) {
	return func(update app_provider.Progress) {
		select {
		case progress <- update:
			return
		default:
		}
		select {
		case <-progress:
		default:
		}
		select {
		case progress <- update:
		default:
		}
	}
}

func (m *Model) readScanProgress() {
	if m.scanProgressCh == nil {
		return
	}
	for {
		select {
		case update := <-m.scanProgressCh:
			m.scanProgress = update
			if update.Warning != "" {
				m.logInfo("扫描提示：" + update.Warning)
			}
		default:
			return
		}
	}
}

func scanProgressTick() tea.Cmd {
	return tea.Tick(10*time.Second, func(time.Time) tea.Msg {
		return scanProgressTickMsg{}
	})
}

func (m Model) updateSelection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.discovered) == 0 {
		if msg.String() == "esc" || msg.String() == "enter" {
			m.screen = homeScreen
		}
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.screen = homeScreen
	case "up", "k":
		if m.selectCursor > 0 {
			m.selectCursor--
		}
	case "down", "j":
		if m.selectCursor < len(m.discovered)-1 {
			m.selectCursor++
		}
	case " ":
		path := m.discovered[m.selectCursor].RootDir
		m.selected[path] = !m.selected[path]
	case "enter":
		apps := make([]storeRepo.TargetApp, 0, len(m.discovered))
		for _, item := range m.discovered {
			if m.selected[item.RootDir] {
				apps = append(apps, storeRepo.TargetApp{RootDir: item.RootDir, AppType: item.AppType})
			}
		}
		if len(apps) == 0 {
			m.status = "请至少勾选一个应用"
			m.logInfo(m.status)
			return m, nil
		}
		if m.repo == nil {
			m.status = "数据库未初始化"
			m.logInfo(m.status)
			return m, nil
		}
		if err := m.repo.Add(apps); err != nil {
			m.status = "保存失败：" + err.Error()
			m.logInfo(m.status)
			return m, nil
		}
		m.apps = m.loadApps()
		m.status = fmt.Sprintf("成功保存 %d 个应用", len(apps))
		m.logInfo(m.status)
		m.screen = homeScreen
	}
	return m, nil
}

// logBriefMessageRunes 限制界面日志单条的字符数，避免长错误占满日志区域。
const logBriefMessageRunes = 100

// logInfo 将状态消息交给统一 logger；logger 的 TUI sink 负责更新界面日志。
func (m *Model) logInfo(message string) {
	if m.logger != nil {
		m.logger.Info("%s", message)
	}
}

// briefLogMessage 生成界面显示用的简要信息：只保留首行并限制长度。
// 命令输出的详细堆栈写入日志文件，界面仅提示查看日志，避免多行输出打乱布局。
func briefLogMessage(message string) string {
	firstLine := strings.TrimSpace(strings.Split(message, "\n")[0])
	runes := []rune(firstLine)
	switch {
	case len(runes) > logBriefMessageRunes:
		return string(runes[:logBriefMessageRunes]) + "…（详见日志文件）"
	case firstLine != strings.TrimSpace(message):
		return firstLine + "…（详见日志文件）"
	default:
		return firstLine
	}
}

func (m *Model) appendDisplayLog(message string) {
	m.logs = append(m.logs, time.Now().Format("15:04:05")+" "+briefLogMessage(message))
	m.logScroll = 0
	if len(m.logs) > 100 {
		m.logs = m.logs[len(m.logs)-100:]
	}
}

func (m *Model) scrollLogs(direction int) {
	m.logScroll += direction * logPageSize
	if m.logScroll < 0 {
		m.logScroll = 0
	}
	maxOffset := len(m.logs) - logPageSize
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.logScroll > maxOffset {
		m.logScroll = maxOffset
	}
}

func (m Model) View() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	displayItems := append(append([]string{}, menuItems...), "更新版本")
	menu := make([]string, 0, len(displayItems))
	for i, item := range displayItems {
		prefix := "  "
		if i == m.menuCursor {
			prefix = "> "
		}
		menu = append(menu, prefix+item)
	}
	updateVersion := ""
	if m.updateResult != nil && m.updateResult.Version != version.String() {
		updateVersion = m.updateResult.Version
	}
	header := renderTitleWithUpdate(width, updateVersion)
	top := "\n" + header
	helpText := menuHelp(m.menuCursor)
	if m.screen == scheduledActionsScreen {
		helpText = scheduledActionHelp(m.selectCursor)
	}
	menuContent := strings.Join(menu, "    ") + "\n" + helpText
	menuView := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1).Width(width - 5).MaxWidth(width - 3).Render(menuContent)

	var center string
	switch m.screen {
	case rootInputScreen:
		if m.scanning {
			center = "扫描根目录\n\n正在扫描，请稍候…"
		} else {
			center = "扫描根目录\n\n" + m.rootInput.View() + "\n\n回车开始扫描 · Esc 返回"
		}
	case selectionScreen:
		rows := make([]string, 0, len(m.discovered))
		for i, item := range m.discovered {
			mark := "[ ]"
			if m.selected[item.RootDir] {
				mark = "[x]"
			}
			cursor := "  "
			if i == m.selectCursor {
				cursor = "> "
			}
			rows = append(rows, fmt.Sprintf("%s%s [%s] %s", cursor, mark, item.AppType, item.RootDir))
		}
		center = "扫描到的应用安装目录（空格勾选，回车保存）\n\n" + strings.Join(rows, "\n")
	case appManagementScreen:
		rootWidth := applicationRootColumnWidth(width)
		rows := []string{applicationTableHeader(rootWidth), applicationTableSeparator(rootWidth)}
		start, end := tableWindow(len(m.apps), m.appManageCursor, m.appManageOffset, tableVisibleRows(m.height, len(m.apps)))
		for i := start; i < end; i++ {
			app := m.apps[i]
			cursor := "  "
			if i == m.appManageCursor {
				cursor = "> "
			}
			rows = append(rows, cursor+formatApplicationRow(app, rootWidth))
		}
		if len(m.apps) == 0 {
			rows = append(rows, "暂无已登记应用")
		}
		center = fmt.Sprintf("应用管理\n\n回车查看站点 · d 删除应用 · Esc 返回 · %s\n\n%s", tableWindowLabel(start, end, len(m.apps)), strings.Join(rows, "\n"))
	case siteListScreen:
		configWidth := siteConfigColumnWidth(width)
		rows := []string{siteTableHeader(configWidth), siteTableSeparator(configWidth)}
		start, end := tableWindow(len(m.managedSites), m.siteManageCursor, m.siteManageOffset, tableVisibleRows(m.height, len(m.managedSites)))
		for i := start; i < end; i++ {
			site := m.managedSites[i]
			cursor := "  "
			if i == m.siteManageCursor {
				cursor = "> "
			}
			state := "禁用"
			if site.Enabled {
				state = "启用"
			}
			https := "否"
			if site.Https {
				https = "是"
			}
			rows = append(rows, fmt.Sprintf("%s%s %s %s %s %s %s",
				cursor,
				fixedColumn(fmt.Sprintf("%d", site.ID), 6),
				fixedColumn(state, 4),
				fixedColumn(site.PrimaryDomain, 26),
				fixedColumn(fmt.Sprintf("%d", site.SubdomainCount), 8),
				fixedColumn(https, 5),
				fixedColumn(site.ConfigPath, configWidth),
			))
		}
		if len(m.managedSites) == 0 {
			rows = append(rows, "该应用暂无已扫描站点")
		}
		center = fmt.Sprintf("站点列表：%s\n\n上下键选择 · 空格启用/禁用 · Esc 返回 · %s\n\n%s", m.managedApp.RootDir, tableWindowLabel(start, end, len(m.managedSites)), strings.Join(rows, "\n"))
	case deleteAppConfirmScreen:
		center = fmt.Sprintf("确认删除应用\n\n%s\n\n该应用及其 %d 个站点记录将被删除。\n\n按 y 确认，按 Esc 取消", m.managedApp.RootDir, m.managedApp.SiteCount)
	case certdSettingsScreen:
		center = "Certd 接口设置\n\nBaseURL\n" + m.certdInputs[0].View() + "\n\nKeyId\n" + m.certdInputs[1].View() + "\n\nKeySecret\n" + m.certdInputs[2].View() + "\n\n本机名称（可选）\n" + m.certdInputs[3].View() + "\n\n最长等待时长（分钟，默认 10）\n" + m.certdInputs[4].View() + "\n\nTab/上下键切换输入框 · Enter 保存 · Esc 返回"
	case scheduleSettingsScreen:
		center = m.viewScheduleSettings()
	case serviceConfirmScreen:
		center = "启动后台定时同步服务\n\n将把客户端注册为系统服务（Windows 服务 / Linux systemd）并开机自启。\n服务会读取“定时设置”中的 Cron 表达式，在后台周期执行站点扫描与证书同步。\n已注册则仅启动，不会重复注册。\n\n当前定时计划：" + m.scheduleSummary() + "\n\n按 y 确认，按 Esc 取消"
	case scheduledActionsScreen:
		rows := make([]string, 0, len(scheduledActionItems))
		for i, item := range scheduledActionItems {
			prefix := "  "
			if i == m.selectCursor {
				prefix = "> "
			}
			rows = append(rows, prefix+item)
		}
		center = "定时执行\n\n" + strings.Join(rows, "\n") + "\n\n上下键选择 · Enter 执行 · Esc 返回"
	default:
		rootWidth := applicationRootColumnWidth(width)
		rows := []string{applicationTableHeader(rootWidth), applicationTableSeparator(rootWidth)}
		for _, app := range m.apps {
			rows = append(rows, formatApplicationRow(app, rootWidth))
		}
		if len(m.apps) == 0 {
			rows = append(rows, "暂无已扫描应用，请从左上菜单选择“应用扫描”")
		}
		center = registeredApplicationsTitle(m.apps) + "\n\n" + strings.Join(rows, "\n")
	}
	renderCenterView := func(content string) string {
		return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1).Width(width - 5).MaxWidth(width - 3).Render(content)
	}
	centerView := renderCenterView(center)
	logEnd := len(m.logs) - m.logScroll
	if logEnd < 0 {
		logEnd = 0
	}
	if logEnd > len(m.logs) {
		logEnd = len(m.logs)
	}
	logStart := logEnd - logPageSize
	if logStart < 0 {
		logStart = 0
	}
	logLines := m.logs[logStart:logEnd]
	totalLogPages := (len(m.logs) + logPageSize - 1) / logPageSize
	if totalLogPages == 0 {
		totalLogPages = 1
	}
	olderPages := 0
	if logStart > 0 {
		olderPages = (logStart + logPageSize - 1) / logPageSize
	}
	currentLogPage := totalLogPages - olderPages
	logHeader := executionLogHeader(width, currentLogPage, totalLogPages)
	status := m.status
	if status == "" {
		status = "←→ 选择菜单 · Enter 确认 · q 退出"
	}
	status = singleLine(status)
	renderLogView := func(lines []string) string {
		wrappedLines := wrapLogLines(lines, width-8)
		return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1).Foreground(lipgloss.Color("244")).Width(width - 5).MaxWidth(width - 3).Render(
			fmt.Sprintf("%s\n\n%s", logHeader, strings.Join(wrappedLines, "\n")),
		)
	}
	logView := renderLogView(logLines)
	if m.height > 0 {
		for visible := len(logLines); visible >= 0; visible-- {
			displayLines := logLines
			if visible < len(displayLines) {
				displayLines = displayLines[len(displayLines)-visible:]
			}
			candidate := renderLogView(displayLines)
			if lipgloss.Height(top+"\n"+menuView+"\n"+centerView+"\n"+candidate+"\n"+status) <= m.height || visible == 0 {
				logView = candidate
				break
			}
		}
		centerLines := strings.Split(center, "\n")
		for lipgloss.Height(top+"\n"+menuView+"\n"+centerView+"\n"+logView+"\n"+status) > m.height && len(centerLines) > 1 {
			centerLines = centerLines[:len(centerLines)-1]
			if len(centerLines) > 1 {
				centerLines[len(centerLines)-1] = "…"
			}
			centerView = renderCenterView(strings.Join(centerLines, "\n"))
		}
	}
	view := top + "\n" + menuView + "\n" + centerView + "\n" + logView + "\n" + status
	return fitViewWidth(view, width)
}

func menuHelp(index int) string {
	help := []string{
		"扫描本机 Nginx、Apache 和 IIS 的应用安装目录",
		"扫描已登记应用的站点与证书配置",
		"查看、删除应用，或启用和禁用站点",
		"设置 Certd 地址、授权信息、本机名称和等待时长",
		"检查 Certd 证书并部署到已启用的 HTTPS 站点",
		"启动、配置、停止或卸载定时同步系统服务",
		"用系统默认程序打开日志文件 logs/client.log，查看详细错误",
		"发现新版本时仅提示，不会自动更新；请通过安装脚本或发布页手动更新",
	}
	if index < 0 || index >= len(help) {
		return ""
	}
	return help[index]
}

func scheduledActionHelp(index int) string {
	help := []string{
		"检查服务状态；未注册时先注册，再启动定时同步服务",
		"设置 Cron 表达式与启用状态，供定时服务使用",
		"停止当前已注册的定时同步服务",
		"从系统中卸载定时同步服务",
		"打开系统服务管理面板或显示 systemctl 操作提示",
	}
	if index < 0 || index >= len(help) {
		return ""
	}
	return help[index]
}

func serviceActionSuccess(action string) string {
	switch action {
	case "ensure":
		return "定时同步服务已启动，您可以按 Ctrl+C 退出本应用界面\n使用 tail -f -n 50 ./logs/client.log 查看运行日志"
	case "stop":
		return "定时同步服务已停止"
	case "uninstall":
		return "定时同步服务已卸载"
	default:
		return "服务操作已完成"
	}
}

func serviceActionFailure(action, reason string) string {
	name := "服务操作"
	switch action {
	case "ensure":
		name = "启动后台定时同步服务"
	case "stop":
		name = "停止定时同步服务"
	case "uninstall":
		name = "卸载定时同步服务"
	}
	return name + "失败：" + reason
}

func executionLogHeader(width, current, total int) string {
	title := "执行日志（PageUp/PageDown 翻页）"
	info := fmt.Sprintf("滚动位置 %s %d/%d", logScrollbar(current, total), current, total)
	contentWidth := width - 8
	gap := contentWidth - lipgloss.Width(title) - lipgloss.Width(info)
	if gap < 1 {
		gap = 1
	}
	return title + strings.Repeat(" ", gap) + info
}

func logScrollbar(current, total int) string {
	const width = 10
	if total < 1 {
		total = 1
	}
	position := (current - 1) * width / total
	if position >= width {
		position = width - 1
	}
	bar := make([]rune, width)
	for i := range bar {
		bar[i] = '░'
	}
	bar[position] = '█'
	return string(bar)
}

func formatApplicationRow(app storeRepo.TargetApp, rootWidth int) string {
	appType := app.AppType
	if !app.Enabled {
		appType += "（禁用）"
	}
	return strings.Join([]string{
		fixedColumn(fmt.Sprintf("%d", app.ID), applicationIDWidth),
		fixedColumn(appType, applicationTypeWidth),
		fixedColumn(app.RootDir, rootWidth),
		fixedColumn(fmt.Sprintf("%d", app.SiteCount), applicationSiteWidth),
		fixedColumn(fmt.Sprintf("%d", app.HttpsSiteCount), applicationHTTPSWidth),
		fixedColumn(fmt.Sprintf("%d", app.SyncedSiteCount), applicationSyncedWidth),
		fixedColumn(fmt.Sprintf("%d", app.FailedSiteCount), applicationFailedWidth),
		fixedColumn(applicationSyncStatus(app), applicationStatusWidth),
	}, " ")
}

func applicationTableHeader(rootWidth int) string {
	return strings.Join([]string{
		fixedColumn("ID", applicationIDWidth),
		fixedColumn("类型", applicationTypeWidth),
		fixedColumn("安装目录", rootWidth),
		fixedColumn("站点数", applicationSiteWidth),
		fixedColumn("HTTPS站点数", applicationHTTPSWidth),
		fixedColumn("已同步", applicationSyncedWidth),
		fixedColumn("异常", applicationFailedWidth),
		fixedColumn("状态", applicationStatusWidth),
	}, " ")
}

func renderTitle(width int) string {
	return renderTitleWithUpdate(width, "")
}

func renderTitleWithUpdate(width int, updateVersion string) string {
	brand := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81")).Render("Certd Client")
	divider := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("  ·  ")
	subtitle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Render("证书管理工具客户端")
	versionText := "v" + version.String()
	if updateVersion != "" {
		versionText += "（有更新 v" + updateVersion + "）"
	}
	build := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("  ·  " + versionText)
	title := brand + divider + subtitle + build
	availableWidth := width - 1
	padding := (availableWidth - lipgloss.Width(title)) / 2
	if padding < 0 {
		padding = 0
	}
	return strings.Repeat(" ", padding) + title
}

func applicationTableSeparator(rootWidth int) string {
	return strings.Repeat("─", lipgloss.Width(applicationTableHeader(rootWidth)))
}

func siteTableSeparator(configWidth int) string {
	return strings.Repeat("─", lipgloss.Width(siteTableHeader(configWidth)))
}

func registeredApplicationsTitle(apps []storeRepo.TargetApp) string {
	var httpsSites, failedSites int
	for _, app := range apps {
		httpsSites += app.HttpsSiteCount
		failedSites += app.FailedSiteCount
	}
	return fmt.Sprintf("已登记应用【HTTPS站点数：%d，异常：%d】", httpsSites, failedSites)
}

func applicationRootColumnWidth(width int) int {
	rootWidth := width - 69
	if rootWidth < 12 {
		return 12
	}
	if rootWidth > 120 {
		return 120
	}
	return rootWidth
}

func applicationSyncStatus(app storeRepo.TargetApp) string {
	icon := "!"
	color := lipgloss.Color("11")
	if app.SyncedSiteCount == app.HttpsSiteCount && app.FailedSiteCount == 0 {
		icon = "✓"
		color = lipgloss.Color("10")
	}
	return lipgloss.NewStyle().Bold(true).Foreground(color).Padding(0, 1).Render(icon)
}

// 计算管理表格可见行数，为标题、操作提示、日志和状态行预留空间。
func tableVisibleRows(height, total int) int {
	if total <= 0 {
		return 0
	}
	if height <= 0 {
		return total
	}
	const fixedHeight = 24
	visible := height - fixedHeight
	if visible < 1 {
		visible = 1
	}
	if visible > total {
		visible = total
	}
	return visible
}

func keepTableCursorVisible(cursor, total, height, offset int) int {
	visible := tableVisibleRows(height, total)
	return keepTableOffsetVisible(cursor, total, visible, offset)
}

func keepTableOffsetVisible(cursor, total, visible, offset int) int {
	if visible <= 0 {
		return 0
	}
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+visible {
		offset = cursor - visible + 1
	}
	maxOffset := total - visible
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	return offset
}

func tableWindow(total, cursor, offset, visible int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if visible <= 0 || visible > total {
		visible = total
	}
	offset = keepTableOffsetVisible(cursor, total, visible, offset)
	if offset > total-visible {
		offset = total - visible
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + visible
	if end > total {
		end = total
	}
	return offset, end
}

func tableWindowLabel(start, end, total int) string {
	if total == 0 {
		return "暂无数据"
	}
	return fmt.Sprintf("第 %d-%d/%d 行", start+1, end, total)
}

func siteTableHeader(configWidth int) string {
	return strings.Join([]string{
		"  " + fixedColumn("ID", 6),
		fixedColumn("状态", 4),
		fixedColumn("主域名", 26),
		fixedColumn("子域名数", 8),
		fixedColumn("HTTPS", 5),
		fixedColumn("配置文件", configWidth),
	}, " ")
}

func siteConfigColumnWidth(width int) int {
	configWidth := width - 66
	if configWidth < 16 {
		return 16
	}
	if configWidth > 160 {
		return 160
	}
	return configWidth
}

func fixedColumn(value string, width int) string {
	if lipgloss.Width(value) > width {
		const ellipsis = "..."
		ellipsisWidth := lipgloss.Width(ellipsis)
		trimmed := make([]rune, 0, width)
		used := 0
		for _, character := range value {
			characterWidth := lipgloss.Width(string(character))
			if used+characterWidth+ellipsisWidth > width {
				break
			}
			trimmed = append(trimmed, character)
			used += characterWidth
		}
		value = string(trimmed) + ellipsis
	}
	padding := width - lipgloss.Width(value)
	if padding < 0 {
		padding = 0
	}
	return value + strings.Repeat(" ", padding)
}

func wrapLogLines(lines []string, width int) []string {
	if width < 1 {
		width = 1
	}
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		current := strings.Builder{}
		used := 0
		for _, character := range line {
			if character == '\n' {
				wrapped = append(wrapped, current.String())
				current.Reset()
				used = 0
				continue
			}
			characterWidth := lipgloss.Width(string(character))
			if used > 0 && used+characterWidth > width {
				wrapped = append(wrapped, current.String())
				current.Reset()
				used = 0
			}
			current.WriteRune(character)
			used += characterWidth
		}
		wrapped = append(wrapped, current.String())
	}
	return wrapped
}

// 外部多行输出会让终端自动换行并破坏增量绘制，因此所有行额外保留最后一列。
func fitViewWidth(view string, width int) string {
	maxWidth := width - 1
	if maxWidth < 1 {
		maxWidth = 1
	}
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if lipgloss.Width(line) >= width {
			lines[i] = lipgloss.NewStyle().MaxWidth(maxWidth).Render(line)
		}
	}
	return strings.Join(lines, "\n")
}

func singleLine(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "\r", " ")
}
