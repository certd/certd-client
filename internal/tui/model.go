package tui

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/store"
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
)

const logPageSize = 5

type Model struct {
	repo            *store.TargetAppRepository
	providers       *app_provider.Registry
	logger          *log.Logger
	menuCursor      int
	screen          screen
	rootInput       textinput.Model
	discovered      []app_provider.App
	selected        map[string]bool
	selectCursor    int
	apps            []store.TargetApp
	appManageCursor int
	managedApp      store.TargetApp
	managedSites    []store.AppSite
	logs            []string
	logScroll       int
	scanning        bool
	siteScanning    bool
	scanProgress    app_provider.Progress
	scanProgressCh  chan app_provider.Progress
	status          string
	width, height   int
}

var menuItems = []string{"应用扫描", "扫描站点", "应用管理"}

func NewModel(repo *store.TargetAppRepository, logger *log.Logger, registries ...*app_provider.Registry) Model {
	input := textinput.New()
	input.Placeholder = "例如 /etc 或 C:\\Web"
	input.CharLimit = 2048
	input.Width = 60
	var providers *app_provider.Registry
	if len(registries) > 0 {
		providers = registries[0]
	}
	return Model{repo: repo, providers: providers, logger: logger, rootInput: input, selected: make(map[string]bool)}
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg { return appsLoadedMsg{apps: m.loadApps()} }
}

type appsLoadedMsg struct {
	apps []store.TargetApp
}

type scanCompletedMsg struct {
	apps []app_provider.App
	err  error
}

type scanProgressTickMsg struct{}

type siteScanCompletedMsg struct {
	appCount  int
	siteCount int
	errors    []string
}

func (m Model) loadApps() []store.TargetApp {
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
		if len(msg.errors) == 0 {
			m.status = fmt.Sprintf("站点扫描完成：扫描 %d 个应用，发现 %d 个站点", msg.appCount, msg.siteCount)
		} else {
			m.status = fmt.Sprintf("站点扫描完成：扫描 %d 个应用，发现 %d 个站点，失败 %d 个", msg.appCount, msg.siteCount, len(msg.errors))
		}
		m.appendLog(m.status)
		for _, scanErr := range msg.errors {
			m.appendLog("站点扫描失败：" + scanErr)
		}
	case tea.KeyMsg:
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
		if m.menuCursor == 0 {
			m.screen = rootInputScreen
			m.rootInput.Reset()
			m.rootInput.Focus()
			m.status = "请输入扫描根目录，回车开始扫描"
			m.appendLog("开始应用扫描：等待输入根目录")
		} else if m.menuCursor == 1 {
			if m.scanning || m.siteScanning {
				m.status = "已有扫描任务正在执行"
				return m, nil
			}
			m.apps = m.loadApps()
			activeApps := make([]store.TargetApp, 0, len(m.apps))
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
		} else {
			m.apps = m.loadApps()
			m.appManageCursor = 0
			m.managedSites = nil
			m.screen = appManagementScreen
			m.status = fmt.Sprintf("应用管理：当前已登记 %d 个应用", len(m.apps))
			m.appendLog(m.status)
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
		}
	case "down", "j":
		if m.appManageCursor < len(m.apps)-1 {
			m.appManageCursor++
		}
	case "enter":
		if m.repo == nil {
			m.status = "数据库未初始化"
			m.appendLog(m.status)
			return m, nil
		}
		m.managedApp = m.apps[m.appManageCursor]
		sites, err := m.repo.ListSites(m.managedApp.ID)
		if err != nil {
			m.status = "读取站点失败：" + err.Error()
			m.appendLog(m.status)
			return m, nil
		}
		m.managedSites = sites
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

func (m Model) scanSites(apps []store.TargetApp) tea.Cmd {
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
			records := make([]store.AppSite, 0, len(sites))
			for _, site := range sites {
				records = append(records, store.AppSite{
					PrimaryDomain:  site.PrimaryDomain,
					SubdomainCount: site.SubdomainCount,
					ConfigPath:     site.ConfigPath,
					Https:          site.Https,
				})
			}
			if err := m.repo.SyncSites(app.ID, records); err != nil {
				result.errors = append(result.errors, fmt.Sprintf("%s：%v", app.RootDir, err))
				continue
			}
			result.siteCount += len(records)
		}
		return result
	}
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
		apps := make([]store.TargetApp, 0, len(m.discovered))
		for _, item := range m.discovered {
			if m.selected[item.RootDir] {
				apps = append(apps, store.TargetApp{RootDir: item.RootDir, AppType: item.AppType})
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
	if width < 60 {
		width = 60
	}
	menu := make([]string, 0, len(menuItems))
	for i, item := range menuItems {
		prefix := "  "
		if i == m.menuCursor {
			prefix = "▶ "
		}
		menu = append(menu, prefix+item)
	}
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("Certd Client · 本地应用管理")
	menuView := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(width - 4).Render(strings.Join(menu, "    "))

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
				cursor = "▶ "
			}
			rows = append(rows, fmt.Sprintf("%s%s [%s] %s", cursor, mark, item.AppType, item.RootDir))
		}
		center = "扫描到的应用安装目录（空格勾选，回车保存）\n\n" + strings.Join(rows, "\n")
	case appManagementScreen:
		rows := []string{"类型      安装目录                                        站点数  HTTPS站点数"}
		for i, app := range m.apps {
			cursor := "  "
			if i == m.appManageCursor {
				cursor = "▶ "
			}
			rows = append(rows, cursor+formatApplicationRow(app))
		}
		if len(m.apps) == 0 {
			rows = append(rows, "暂无已登记应用")
		}
		center = "应用管理\n\n回车查看站点 · d 删除应用 · Esc 返回\n\n" + strings.Join(rows, "\n")
	case siteListScreen:
		rows := []string{"主域名                     子域名数  HTTPS  配置文件"}
		for _, site := range m.managedSites {
			https := "否"
			if site.Https {
				https = "是"
			}
			rows = append(rows, fmt.Sprintf("%-26s %8d  %-5s  %s", site.PrimaryDomain, site.SubdomainCount, https, site.ConfigPath))
		}
		if len(m.managedSites) == 0 {
			rows = append(rows, "该应用暂无已扫描站点")
		}
		center = "站点列表：" + m.managedApp.RootDir + "\n\n" + strings.Join(rows, "\n")
	case deleteAppConfirmScreen:
		center = fmt.Sprintf("确认删除应用\n\n%s\n\n该应用及其 %d 个站点记录将被删除。\n\n按 y 确认，按 Esc 取消", m.managedApp.RootDir, m.managedApp.SiteCount)
	default:
		rows := []string{"类型      安装目录                                        站点数  HTTPS站点数"}
		for _, app := range m.apps {
			rows = append(rows, formatApplicationRow(app))
		}
		if len(m.apps) == 0 {
			rows = append(rows, "暂无已扫描应用，请从左上菜单选择“应用扫描”")
		}
		center = "已登记应用\n\n" + strings.Join(rows, "\n")
	}
	centerView := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1).Width(width - 4).Render(center)
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
	logView := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1).Foreground(lipgloss.Color("244")).Width(width - 4).Render("执行日志\n\n" + strings.Join(logLines, "\n"))
	status := m.status
	if status == "" {
		status = "←→ 选择菜单 · Enter 确认 · q 退出"
	}
	return header + "\n" + menuView + "\n" + centerView + "\n" + logView + "\n" + status
}

func formatApplicationRow(app store.TargetApp) string {
	appType := app.AppType
	if !app.Enabled {
		appType += "（禁用）"
	}
	return fmt.Sprintf("%-12s %-48s %6d  %10d", appType, app.RootDir, app.SiteCount, app.HttpsSiteCount)
}
