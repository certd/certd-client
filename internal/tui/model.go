package tui

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"

	"github.com/certd/certd-client/internal/app_provider"
	storeRepo "github.com/certd/certd-client/internal/store/repo"
	"github.com/certd/certd-client/internal/version"
	"github.com/charmbracelet/bubbles/key"
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
	repo             *storeRepo.TargetAppRepository
	siteRepo         *storeRepo.AppSiteRepository
	settingsRepo     *storeRepo.SettingsRepository
	providers        *app_provider.Registry
	logger           *log.Logger
	menuCursor       int
	screen           screen
	rootInput        textinput.Model
	discovered       []app_provider.App
	selected         map[string]bool
	selectCursor     int
	apps             []storeRepo.TargetApp
	appManageCursor  int
	appManageOffset  int
	managedApp       storeRepo.TargetApp
	managedSites     []storeRepo.AppSite
	siteManageCursor int
	siteManageOffset int
	logs             []string
	logScroll        int
	scanning         bool
	siteScanning     bool
	scanProgress     app_provider.Progress
	scanProgressCh   chan app_provider.Progress
	status           string
	certdInputs      [5]textinput.Model
	certdFocus       int
	syncing          bool
	syncProgressCh   chan string
	syncCancel       context.CancelFunc
	startRequested   bool
	width, height    int
}

var menuItems = []string{"应用扫描", "站点扫描", "应用管理", "Certd接口设置", "同步证书", "定时同步"}

func NewModel(repo *storeRepo.TargetAppRepository, siteRepo *storeRepo.AppSiteRepository, logger *log.Logger, registries ...*app_provider.Registry) Model {
	return NewModelWithSettings(repo, siteRepo, nil, logger, registries...)
}

func NewModelWithSettings(repo *storeRepo.TargetAppRepository, siteRepo *storeRepo.AppSiteRepository, settingsRepo *storeRepo.SettingsRepository, logger *log.Logger, registries ...*app_provider.Registry) Model {
	input := textinput.New()
	input.Placeholder = "例如 /etc 或 C:\\Web"
	input.CharLimit = 2048
	input.Width = 60
	configureInputForPlatform(&input, runtime.GOOS)
	var providers *app_provider.Registry
	if len(registries) > 0 {
		providers = registries[0]
	}
	certdInputs := [5]textinput.Model{}
	for i, placeholder := range []string{"https://certd.example.com", "keyId", "keySecret", "本机名称（可选）", "10"} {
		certdInputs[i] = textinput.New()
		certdInputs[i].Placeholder = placeholder
		certdInputs[i].CharLimit = 2048
		certdInputs[i].Width = 60
		configureInputForPlatform(&certdInputs[i], runtime.GOOS)
	}
	certdInputs[2].EchoMode = textinput.EchoPassword
	return Model{
		repo: repo, siteRepo: siteRepo, settingsRepo: settingsRepo, providers: providers, logger: logger,
		rootInput: input, certdInputs: certdInputs, selected: make(map[string]bool),
	}
}

// configureInputForPlatform 根据平台调整文本输入框的键盘绑定。
// macOS 上 textinput 把 Ctrl+V 绑定到剪贴板粘贴，粘贴会通过 pbpaste 子进程读取系统剪贴板，
// 该子进程在 TUI 的 raw 模式下可能导致程序闪退。macOS 终端的 Cmd+V 由终端直接注入文本，
// 不经过 pbpaste，因此禁用 Ctrl+V，粘贴统一走 Cmd+V。
func configureInputForPlatform(input *textinput.Model, goos string) {
	if goos == "darwin" {
		input.KeyMap.Paste = key.NewBinding(key.WithDisabled())
	}
}

// certdSettingsPasteHint 返回粘贴提示；macOS 上 Ctrl+V 已禁用，提示使用 Cmd+V。
func certdSettingsPasteHint() string {
	if runtime.GOOS == "darwin" {
		return " · macOS 请用 Cmd+V 粘贴"
	}
	return ""
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg { return appsLoadedMsg{apps: m.loadApps()} }
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
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case appsLoadedMsg:
		m.apps = msg.apps
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
		m.appendLog(m.status)
		return m, scanProgressTick()
	case scanCompletedMsg:
		m.scanning = false
		m.scanProgressCh = nil
		if msg.err != nil {
			m.status = "扫描失败：" + msg.err.Error()
			m.appendLog(m.status)
			return m, nil
		}
		m.discovered = msg.apps
		m.selected = make(map[string]bool, len(msg.apps))
		m.selectCursor = 0
		m.screen = selectionScreen
		m.status = fmt.Sprintf("扫描完成，发现 %d 个应用安装目录", len(msg.apps))
		m.appendLog(m.status)
	case siteScanCompletedMsg:
		m.siteScanning = false
		m.apps = m.loadApps()
		m.status = fmt.Sprintf("站点扫描完成：扫描 %d 个应用，发现 %d 个站点，禁用 %d 个，HTTPS %d 个，新增 %d 个", msg.appCount, msg.siteCount, msg.disabledCount, msg.httpsCount, msg.newCount)
		if len(msg.errors) > 0 {
			m.status += fmt.Sprintf("，失败 %d 个", len(msg.errors))
		}
		m.appendLog(m.status)
		for _, scanErr := range msg.errors {
			m.appendLog("站点扫描失败：" + scanErr)
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
		m.appendLog(m.status)
		for _, item := range msg.result.Errors {
			m.appendLog("证书同步失败：" + item)
		}
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
			m.appendLog(m.status)
			return m, nil
		}
		if msg.String() == "ctrl+c" || (msg.String() == "q" && m.screen != rootInputScreen) {
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
		if m.menuCursor < len(menuItems)-1 {
			m.menuCursor++
		}
	case "enter", " ":
		if m.scanning || m.siteScanning || m.syncing {
			m.status = "已有任务正在执行"
			return m, nil
		}
		if m.menuCursor == 0 {
			m.screen = rootInputScreen
			m.rootInput.Reset()
			m.rootInput.Focus()
			m.status = "请输入扫描根目录，回车开始扫描"
			m.appendLog("开始应用扫描：等待输入根目录")
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
				m.appendLog(m.status)
				return m, nil
			}
			m.siteScanning = true
			m.status = fmt.Sprintf("开始扫描 %d 个已登记应用的站点", len(activeApps))
			m.appendLog(m.status)
			return m, m.scanSites(activeApps)
		} else if m.menuCursor == 2 {
			m.apps = m.loadApps()
			m.appManageCursor = 0
			m.appManageOffset = 0
			m.managedSites = nil
			m.screen = appManagementScreen
			m.status = fmt.Sprintf("应用管理：当前已登记 %d 个应用", len(m.apps))
			m.appendLog(m.status)
		} else if m.menuCursor == 3 {
			m.loadCertdSettings()
			m.certdFocus = 0
			for i := range m.certdInputs {
				m.certdInputs[i].Blur()
			}
			m.certdInputs[0].Focus()
			m.screen = certdSettingsScreen
			m.status = "请输入 Certd 接口配置，回车保存，Esc 返回"
			m.appendLog("打开 Certd 接口设置")
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
				m.appendLog(m.status)
				return m, nil
			}
			m.syncing = true
			m.syncProgressCh = make(chan string, 32)
			syncContext, cancel := context.WithCancel(context.Background())
			m.syncCancel = cancel
			m.status = fmt.Sprintf("开始同步 %d 个应用的证书", len(activeApps))
			m.appendLog(m.status)
			return m, tea.Batch(m.syncCertificatesService(syncContext, activeApps, m.syncProgressCh), syncProgressTick())
		} else {
			m.startRequested = true
			m.appendLog("切换到定时同步模式")
			return m, tea.Quit
		}
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
			m.appendLog(m.status)
			return m, nil
		}
		m.managedApp = m.apps[m.appManageCursor]
		if m.siteRepo == nil {
			m.status = "站点仓库未初始化"
			m.appendLog(m.status)
			return m, nil
		}
		sites, err := m.siteRepo.ListSites(m.managedApp.ID)
		if err != nil {
			m.status = "读取站点失败：" + err.Error()
			m.appendLog(m.status)
			return m, nil
		}
		m.managedSites = sites
		m.siteManageCursor = 0
		m.siteManageOffset = 0
		m.screen = siteListScreen
		m.status = fmt.Sprintf("查看应用站点：%s", m.managedApp.RootDir)
		m.appendLog(m.status)
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
			m.appendLog(m.status)
			return m, nil
		}
		site := &m.managedSites[m.siteManageCursor]
		enabled := !site.Enabled
		if err := m.siteRepo.SetEnabled(site.ID, enabled); err != nil {
			m.status = "更新站点状态失败：" + err.Error()
			m.appendLog(m.status)
			return m, nil
		}
		site.Enabled = enabled
		if enabled {
			m.status = "已启用站点：" + site.PrimaryDomain
		} else {
			m.status = "已禁用站点：" + site.PrimaryDomain
		}
		m.appendLog(m.status)
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
			m.appendLog(m.status)
			return m, nil
		}
		deletedSites, err := m.repo.DeleteApp(m.managedApp.ID)
		if err != nil {
			m.status = "删除应用失败：" + err.Error()
			m.appendLog(m.status)
			return m, nil
		}
		m.apps = m.loadApps()
		if m.appManageCursor >= len(m.apps) && m.appManageCursor > 0 {
			m.appManageCursor--
		}
		m.managedSites = nil
		m.screen = appManagementScreen
		m.status = fmt.Sprintf("已删除应用及 %d 个站点", deletedSites)
		m.appendLog(m.status)
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
			m.appendLog("扫描失败：根目录为空")
			return m, nil
		}
		if m.providers == nil || len(m.providers.All()) == 0 {
			m.status = "未注册应用扫描 Provider"
			m.appendLog(m.status)
			return m, nil
		}
		if m.repo != nil {
			disabledApps, err := m.repo.DisableMissingApps()
			if err != nil {
				m.status = "检查已登记应用失败：" + err.Error()
				m.appendLog(m.status)
				return m, nil
			}
			if len(disabledApps) > 0 {
				m.apps = m.loadApps()
				m.appendLog(fmt.Sprintf("已禁用 %d 个不存在的应用目录", len(disabledApps)))
			}
		}
		m.scanning = true
		m.scanProgress = app_provider.Progress{RemainingDirectories: 1}
		m.scanProgressCh = make(chan app_provider.Progress, 1)
		m.rootInput.Blur()
		m.status = "开始扫描，请稍候"
		m.appendLog("开始扫描根目录：" + root)
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
				m.appendLog("扫描提示：" + update.Warning)
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
			m.appendLog(m.status)
			return m, nil
		}
		if m.repo == nil {
			m.status = "数据库未初始化"
			m.appendLog(m.status)
			return m, nil
		}
		if err := m.repo.Add(apps); err != nil {
			m.status = "保存失败：" + err.Error()
			m.appendLog(m.status)
			return m, nil
		}
		m.apps = m.loadApps()
		m.status = fmt.Sprintf("成功保存 %d 个应用", len(apps))
		m.appendLog(m.status)
		m.screen = homeScreen
	}
	return m, nil
}

func (m *Model) appendLog(message string) {
	m.logs = append(m.logs, time.Now().Format("15:04:05")+" "+message)
	m.logScroll = 0
	if len(m.logs) > 100 {
		m.logs = m.logs[len(m.logs)-100:]
	}
	if m.logger != nil {
		m.logger.Println(message)
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
	menu := make([]string, 0, len(menuItems))
	for i, item := range menuItems {
		prefix := "  "
		if i == m.menuCursor {
			prefix = "> "
		}
		menu = append(menu, prefix+item)
	}
	header := renderTitle(width)
	top := "\n" + header
	menuContent := strings.Join(menu, "    ") + "\n" + menuHelp(m.menuCursor)
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
		center = "Certd 接口设置\n\nBaseURL\n" + m.certdInputs[0].View() + "\n\nKeyId\n" + m.certdInputs[1].View() + "\n\nKeySecret\n" + m.certdInputs[2].View() + "\n\n本机名称（可选）\n" + m.certdInputs[3].View() + "\n\n最长等待时长（分钟，默认 10）\n" + m.certdInputs[4].View() + "\n\nTab/上下键切换输入框 · Enter 保存 · Esc 返回" + certdSettingsPasteHint()
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
		"退出终端界面并启动定时同步任务",
	}
	if index < 0 || index >= len(help) {
		return ""
	}
	return help[index]
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
	brand := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81")).Render("Certd Client")
	divider := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("  ·  ")
	subtitle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Render("证书管理工具客户端")
	build := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("  ·  v" + version.String())
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
