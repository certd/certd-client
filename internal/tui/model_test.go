package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/app_provider/apache"
	"github.com/certd/certd-client/internal/app_provider/nginx"
	"github.com/certd/certd-client/internal/store"
	storeRepo "github.com/certd/certd-client/internal/store/repo"
	"github.com/certd/certd-client/internal/syncservice"
	"github.com/certd/certd-client/internal/version"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func testRegistry() *app_provider.Registry {
	return app_provider.NewRegistry(nginx.New(), apache.New())
}

type staticProvider struct {
	typeName string
	apps     []app_provider.App
}

func (p staticProvider) Type() string { return p.typeName }

func (p staticProvider) ScanApps(_ string, report func(app_provider.Progress)) ([]app_provider.App, error) {
	if report != nil {
		report(app_provider.Progress{ProviderType: p.typeName, ScannedDirectories: 1})
	}
	return p.apps, nil
}

func (p staticProvider) ScanSites(app_provider.App) ([]app_provider.Site, error) { return nil, nil }

func TestAppendLogAddsDisplayTime(t *testing.T) {
	model := Model{}
	model.appendLog("扫描完成")

	if len(model.logs) != 1 {
		t.Fatalf("expected one log entry, got %d", len(model.logs))
	}
	if !regexp.MustCompile(`^\d{2}:\d{2}:\d{2} 扫描完成$`).MatchString(model.logs[0]) {
		t.Fatalf("expected a time-prefixed log entry, got %q", model.logs[0])
	}
}

func TestLogPagingMovesByOnePageAndReturnsToBottom(t *testing.T) {
	model := Model{}
	for i := 1; i <= 25; i++ {
		model.logs = append(model.logs, fmt.Sprintf("log-%02d", i))
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(Model)
	if model.logScroll != logPageSize {
		t.Fatalf("expected page up to move by %d, got %d", logPageSize, model.logScroll)
	}
	if !strings.Contains(model.View(), "log-07") || strings.Contains(model.View(), "log-25") {
		t.Fatal("page up should show an older log page")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(Model)
	if model.logScroll != 0 {
		t.Fatalf("expected page down to return to bottom, got %d", model.logScroll)
	}
	if !strings.Contains(model.View(), "log-25") {
		t.Fatal("page down should show the latest log page")
	}
}

func TestExecutionLogShowsPagingHintAndScrollbarPosition(t *testing.T) {
	model := Model{width: 120}
	for i := 0; i < 25; i++ {
		model.logs = append(model.logs, fmt.Sprintf("log-%02d", i))
	}
	view := model.View()
	for _, expected := range []string{"PageUp/PageDown 翻页", "滚动位置", "1/3"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected %q in execution log view:\n%s", expected, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "执行日志（") && (!strings.Contains(line, "滚动位置") || !strings.Contains(line, "1/3")) {
			t.Fatalf("scrollbar header wrapped onto another line: %q", line)
		}
	}
}

func TestScanProgressTickWritesProgressLog(t *testing.T) {
	progress := make(chan app_provider.Progress, 1)
	progress <- app_provider.Progress{ScannedDirectories: 42, RemainingDirectories: 7}
	model := Model{scanning: true, scanProgressCh: progress}

	updated, next := model.Update(scanProgressTickMsg{})
	model = updated.(Model)
	if next == nil {
		t.Fatal("expected the next progress tick to be scheduled")
	}
	if !strings.Contains(model.status, "已扫描 42 个目录，剩余 7 个目录") {
		t.Fatalf("unexpected progress status: %q", model.status)
	}
	if len(model.logs) != 1 || !strings.Contains(model.logs[0], "已扫描 42 个目录，剩余 7 个目录") {
		t.Fatalf("expected progress log, got %#v", model.logs)
	}
}

func TestStartingScanWritesLogImmediately(t *testing.T) {
	model := NewModel(nil, nil, nil, testRegistry())
	model.rootInput.SetValue("C:\\scan-root")

	updated, command := model.updateRootInput(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command == nil || !model.scanning {
		t.Fatal("expected scanning command to start")
	}
	if len(model.logs) != 1 || !strings.Contains(model.logs[0], "开始扫描根目录：C:\\scan-root") {
		t.Fatalf("expected immediate scan log, got %#v", model.logs)
	}
}

func TestStartingAppScanDisablesMissingApplications(t *testing.T) {
	db, err := store.OpenDatabase("file:disable-before-scan?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	repo := storeRepo.NewTargetAppRepository(db)
	missingRoot := filepath.Join(t.TempDir(), "missing")
	if err := repo.Add([]storeRepo.TargetApp{{RootDir: missingRoot, AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}
	model := NewModel(repo, nil, nil, testRegistry())
	model.rootInput.SetValue(t.TempDir())

	updated, command := model.updateRootInput(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command == nil || !model.scanning || !strings.Contains(strings.Join(model.logs, "\n"), "已禁用 1 个不存在的应用目录") {
		t.Fatalf("expected disabled-app precheck before scan, got %#v", model)
	}
	apps, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if apps[0].Enabled {
		t.Fatalf("expected missing application to be disabled, got %#v", apps[0])
	}
}

func TestMenuIncludesSiteScan(t *testing.T) {
	if len(menuItems) != 6 || menuItems[1] != "扫描站点" || menuItems[3] != "Certd接口设置" || menuItems[4] != "同步证书" || menuItems[5] != "定时同步" {
		t.Fatalf("expected site scan menu item, got %#v", menuItems)
	}
	if !strings.Contains(menuHelp(5), "定时") {
		t.Fatalf("expected scheduled sync help, got %q", menuHelp(5))
	}
}

func TestScheduledSyncMenuRequestsStartMode(t *testing.T) {
	model := Model{menuCursor: 5}

	updated, command := model.updateHome(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if !model.StartRequested() {
		t.Fatal("expected scheduled sync menu to request start mode")
	}
	if command == nil {
		t.Fatal("expected scheduled sync menu to quit the TUI")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("expected TUI quit command, got %#v", command())
	}
}

func TestViewShowsSelectedMenuHelpWithSquareBorder(t *testing.T) {
	model := NewModel(nil, nil, nil)
	model.width = 100
	model.menuCursor = 4

	view := model.View()

	if !strings.Contains(view, menuHelp(model.menuCursor)) {
		t.Fatalf("expected selected menu help in view: %s", view)
	}
	if strings.Contains(view, "▶") || !strings.Contains(view, "> 同步证书") {
		t.Fatalf("expected a single-column menu cursor: %s", view)
	}
	if strings.Contains(view, "╭") || strings.Contains(view, "╮") {
		t.Fatalf("expected menu without rounded border: %s", view)
	}
}

func TestEscCancelsRunningCertificateSync(t *testing.T) {
	canceled := false
	model := Model{
		syncing:    true,
		syncCancel: func() { canceled = true },
	}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)

	if !canceled || command != nil || model.status != "正在取消证书同步" {
		t.Fatalf("expected Esc to cancel synchronization, canceled=%v status=%q command=%v", canceled, model.status, command)
	}
}

func TestApplicationScanUsesAllRegisteredProviders(t *testing.T) {
	registry := app_provider.NewRegistry(
		staticProvider{typeName: "nginx", apps: []app_provider.App{{RootDir: "C:\\nginx", AppType: "nginx"}}},
		staticProvider{typeName: "apache", apps: []app_provider.App{{RootDir: "C:\\apache", AppType: "apache"}}},
	)
	model := NewModel(nil, nil, nil, registry)
	message := model.scanApps("C:\\", make(chan app_provider.Progress, 1))()
	completed := message.(scanCompletedMsg)
	if completed.err != nil || len(completed.apps) != 2 {
		t.Fatalf("expected applications from every provider, got %#v", completed)
	}
}

func TestApplicationSelectionViewShowsTypeBeforeInstallPath(t *testing.T) {
	model := Model{
		screen: selectionScreen,
		width:  120,
		discovered: []app_provider.App{
			{AppType: "nginx", RootDir: "C:\\nginx"},
			{AppType: "apache", RootDir: "C:\\Apache24"},
			{AppType: "iis", RootDir: "C:\\Windows\\System32\\inetsrv"},
		},
		selected: make(map[string]bool),
	}

	view := model.View()
	for _, expected := range []string{"[nginx] C:\\nginx", "[apache] C:\\Apache24", "[iis] C:\\Windows\\System32\\inetsrv"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected application type before path %q:\n%s", expected, view)
		}
	}
}

func TestRegisteredAppsViewShowsHeadersAndSiteCounts(t *testing.T) {
	model := Model{apps: []storeRepo.TargetApp{{ID: 42, AppType: "nginx", RootDir: "C:\\nginx", Enabled: true, SiteCount: 2, HttpsSiteCount: 1}}}
	view := model.View()
	if !strings.Contains(view, "证书管理工具客户端") {
		t.Fatalf("expected branded title in view:\n%s", view)
	}
	for _, text := range []string{"v" + version.Version, "已登记应用【HTTPS站点数：1，异常：0】", "ID", "类型", "安装目录", "站点数", "HTTPS站点数", "nginx", "42", "C:\\nginx", "2", "1"} {
		if !strings.Contains(view, text) {
			t.Fatalf("expected %q in registered apps view:\n%s", text, view)
		}
	}
	if strings.Contains(view, "0001-01-01") {
		t.Fatalf("registered apps view should not render add time:\n%s", view)
	}
}

func TestRenderTitleUsesCenteredSingleLineWithoutFillingViewport(t *testing.T) {
	title := renderTitle(80)
	lines := strings.Split(title, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected a single-line title, got %q", title)
	}
	for _, expected := range []string{"Certd Client", "证书管理工具客户端", "v" + version.Version} {
		if !strings.Contains(lines[0], expected) {
			t.Fatalf("expected title line %q in %q", expected, lines[0])
		}
	}
	content := strings.TrimLeft(title, " ")
	padding := lipgloss.Width(title) - lipgloss.Width(content)
	wantPadding := (79 - lipgloss.Width(content)) / 2
	if padding != wantPadding || lipgloss.Width(title) >= 80 {
		t.Fatalf("expected centered title without filling the viewport: title=%q padding=%d want=%d", title, padding, wantPadding)
	}
}

func TestViewPlacesMenuDirectlyBelowCenteredTitle(t *testing.T) {
	lines := strings.Split((Model{width: 80}).View(), "\n")
	preview := lines
	if len(preview) > 3 {
		preview = preview[:3]
	}
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "" || !strings.Contains(lines[1], "Certd Client") || strings.TrimSpace(lines[2]) == "" {
		t.Fatalf("expected one blank line above the centered title and the menu directly below it, got %q", preview)
	}
}

func TestViewLeavesLastTerminalColumnUnused(t *testing.T) {
	model := Model{width: 120, menuCursor: 4}

	for _, line := range strings.Split(model.View(), "\n") {
		if lipgloss.Width(line) >= model.width {
			t.Fatalf("rendered line must leave the last terminal column unused: width=%d viewport=%d line=%q", lipgloss.Width(line), model.width, line)
		}
	}
}

func TestRegisteredApplicationsTitleShowsSyncStatus(t *testing.T) {
	title := registeredApplicationsTitle([]storeRepo.TargetApp{{HttpsSiteCount: 3, SyncedSiteCount: 3}})
	if strings.Contains(title, "✓") || strings.Contains(title, "!") {
		t.Fatalf("summary title should not contain an application status icon, got %q", title)
	}

	healthy := formatApplicationRow(storeRepo.TargetApp{HttpsSiteCount: 3, SyncedSiteCount: 3}, 20)
	if !strings.Contains(healthy, "✓") {
		t.Fatalf("expected a check after the application's failure count, got %q", healthy)
	}

	failed := formatApplicationRow(storeRepo.TargetApp{HttpsSiteCount: 3, SyncedSiteCount: 2, FailedSiteCount: 1}, 20)
	if !strings.Contains(failed, "!") {
		t.Fatalf("expected a warning icon after the application's failure count, got %q", failed)
	}
}

func TestApplicationTableSeparatesHeaderFromRows(t *testing.T) {
	model := Model{apps: []storeRepo.TargetApp{{ID: 42, AppType: "nginx", RootDir: "C:\\nginx", Enabled: true}}}
	view := model.View()
	if !strings.Contains(view, "────") {
		t.Fatalf("expected a horizontal separator below the application header:\n%s", view)
	}
}

func TestRegisteredAppsHeaderStaysOnOneLine(t *testing.T) {
	model := Model{width: 80, apps: []storeRepo.TargetApp{{AppType: "nginx", RootDir: "C:\\nginx"}}}
	view := model.View()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "类型") {
			if !strings.Contains(line, "异常") {
				t.Fatalf("application header wrapped: %q\n%s", line, view)
			}
			if lipgloss.Width(line) > 80 {
				t.Fatalf("application header exceeds viewport: width=%d line=%q", lipgloss.Width(line), line)
			}
			return
		}
	}
	t.Fatalf("application header not found:\n%s", view)
}

func TestViewNeverExceedsViewportWidthWithApplicationStatus(t *testing.T) {
	model := Model{
		width: 80,
		apps: []storeRepo.TargetApp{{
			ID:              1,
			AppType:         "nginx",
			RootDir:         `C:\\very-long-installation-directory`,
			Enabled:         true,
			SiteCount:       1,
			HttpsSiteCount:  1,
			SyncedSiteCount: 1,
		}},
	}

	for _, line := range strings.Split(model.View(), "\n") {
		if lipgloss.Width(line) > model.width {
			t.Fatalf("rendered line exceeds viewport: width=%d viewport=%d line=%q", lipgloss.Width(line), model.width, line)
		}
	}
}

func TestViewFitsTerminalHeightAfterScan(t *testing.T) {
	apps := make([]storeRepo.TargetApp, 0, 20)
	for i := 0; i < 20; i++ {
		apps = append(apps, storeRepo.TargetApp{AppType: "nginx", RootDir: fmt.Sprintf("C:\\Data\\very-long-installation-%d", i)})
	}
	model := Model{width: 120, height: 24, apps: apps}
	for i := 0; i < 10; i++ {
		model.logs = append(model.logs, fmt.Sprintf("扫描进度 %d", i))
	}
	view := model.View()
	if lipgloss.Height(view) > model.height {
		t.Fatalf("view overflows terminal height: got %d, want <= %d\n%s", lipgloss.Height(view), model.height, view)
	}
	if !strings.Contains(view, "应用扫描") || !strings.Contains(view, "执行日志") {
		t.Fatalf("expected menu and execution log to remain visible:\n%s", view)
	}
}

func TestExecutionLogWrapsLongEntriesWithinViewport(t *testing.T) {
	model := Model{width: 80, height: 24, logs: []string{strings.Repeat("证书同步失败：读取配置文件详细错误 ", 8)}}
	view := model.View()
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > model.width {
			t.Fatalf("rendered line exceeds viewport width: width=%d line=%q", lipgloss.Width(line), line)
		}
	}
}

func TestApplicationTableShowsIDBeforeTypeAndFixedCountColumns(t *testing.T) {
	rootWidth := 20
	app := storeRepo.TargetApp{ID: 42, AppType: "nginx", RootDir: `C:\\nginx`, Enabled: true, SiteCount: 1, HttpsSiteCount: 12, SyncedSiteCount: 3, FailedSiteCount: 4}
	row := formatApplicationRow(app, rootWidth)
	want := fixedColumn("42", 6) + " " + fixedColumn("nginx", 12) + " " + fixedColumn(app.RootDir, rootWidth) + " " +
		fixedColumn("1", applicationSiteWidth) + " " + fixedColumn("12", applicationHTTPSWidth) + " " + fixedColumn("3", applicationSyncedWidth) + " " + fixedColumn("4", applicationFailedWidth) + " " + fixedColumn(applicationSyncStatus(app), applicationStatusWidth)
	if row != want {
		t.Fatalf("expected ID before application type:\n got %q\nwant %q", row, want)
	}
}

func TestSiteListShowsSiteID(t *testing.T) {
	model := Model{
		screen: siteListScreen,
		managedSites: []storeRepo.AppSite{{
			ID: 24, PrimaryDomain: "example.com", Enabled: true,
		}},
	}
	view := model.View()
	if !strings.Contains(view, "ID") || !strings.Contains(view, "24") {
		t.Fatalf("expected site list to display the site ID:\n%s", view)
	}
}

func TestFixedColumnReservesActualEllipsisWidth(t *testing.T) {
	if got, want := fixedColumn("123456789012345", 12), "123456789..."; got != want {
		t.Fatalf("expected ellipsis to reserve its actual width:\n got %q\nwant %q", got, want)
	}
}

func TestWideTerminalExpandsPathColumns(t *testing.T) {
	if got := applicationRootColumnWidth(220); got <= 48 {
		t.Fatalf("expected a wide terminal to expand the application path column, got %d", got)
	}
	if got := siteConfigColumnWidth(220); got <= 64 {
		t.Fatalf("expected a wide terminal to expand the site config column, got %d", got)
	}
}

func TestApplicationRootWidthReservesStatusColumn(t *testing.T) {
	if got, want := applicationRootColumnWidth(100), 31; got != want {
		t.Fatalf("expected application path width to reserve the status column: got %d want %d", got, want)
	}
}

func TestSiteScanPersistsSitesForRegisteredApplications(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "conf", "site.conf")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("server { listen 443 ssl; server_name example.com www.example.com; }"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenDatabase("file:tui-site-scan?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	repo := storeRepo.NewTargetAppRepository(db)
	siteRepo := storeRepo.NewAppSiteRepository(db)
	if err := repo.Add([]storeRepo.TargetApp{{RootDir: root, AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}
	apps, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}

	model := NewModel(repo, siteRepo, nil, testRegistry())
	updated, _ := model.Update(model.scanSites(apps)())
	model = updated.(Model)
	sites, err := siteRepo.ListSites(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if model.siteScanning || len(sites) != 1 || sites[0].PrimaryDomain != "example.com" || sites[0].SubdomainCount != 1 || !sites[0].Https {
		t.Fatalf("unexpected site scan result: model=%#v sites=%#v", model, sites)
	}
}

func TestSiteScanCompletionShowsDetailedCounts(t *testing.T) {
	model := Model{}
	updated, _ := model.Update(siteScanCompletedMsg{
		appCount:      2,
		siteCount:     15,
		disabledCount: 3,
		httpsCount:    9,
		newCount:      4,
	})
	model = updated.(Model)
	for _, expected := range []string{"发现 15 个站点", "禁用 3 个", "HTTPS 9 个", "新增 4 个"} {
		if !strings.Contains(model.status, expected) {
			t.Fatalf("expected site scan summary to contain %q, got %q", expected, model.status)
		}
	}
}

func TestSummarizeSiteScanCountsDisabledHTTPSAndNewSites(t *testing.T) {
	existing := []storeRepo.AppSite{
		{PrimaryDomain: "disabled.example.com", ConfigPath: "disabled.conf", Enabled: false},
		{PrimaryDomain: "enabled.example.com", ConfigPath: "enabled.conf", Enabled: true},
	}
	discovered := []storeRepo.AppSite{
		{PrimaryDomain: "disabled.example.com", ConfigPath: "disabled.conf", Https: true},
		{PrimaryDomain: "enabled.example.com", ConfigPath: "enabled.conf", Https: false},
		{PrimaryDomain: "new.example.com", ConfigPath: "new.conf", Https: true},
	}
	summary := summarizeSiteScan(existing, discovered)
	if summary.siteCount != 3 || summary.disabledCount != 1 || summary.httpsCount != 2 || summary.newCount != 1 {
		t.Fatalf("unexpected site scan summary: %#v", summary)
	}
}

func TestApplicationManagementViewsSitesAndDeletesApplication(t *testing.T) {
	db, err := store.OpenDatabase("file:application-management?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	repo := storeRepo.NewTargetAppRepository(db)
	siteRepo := storeRepo.NewAppSiteRepository(db)
	if err := repo.Add([]storeRepo.TargetApp{{RootDir: t.TempDir(), AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}
	apps, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if err := siteRepo.SyncSites(apps[0].ID, []storeRepo.AppSite{{PrimaryDomain: "example.com", ConfigPath: "site.conf"}}); err != nil {
		t.Fatal(err)
	}

	model := NewModel(repo, siteRepo, nil, testRegistry())
	model.menuCursor = 2
	updated, _ := model.updateHome(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.screen != appManagementScreen {
		t.Fatalf("expected application management screen, got %v", model.screen)
	}
	updated, _ = model.updateAppManagement(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.screen != siteListScreen || len(model.managedSites) != 1 {
		t.Fatalf("expected site list screen, got %#v", model)
	}
	updated, _ = model.updateSiteList(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	updated, _ = model.updateAppManagement(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	if model.screen != deleteAppConfirmScreen {
		t.Fatalf("expected deletion confirmation, got %v", model.screen)
	}
	updated, _ = model.updateDeleteAppConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)
	if model.screen != appManagementScreen || len(model.apps) != 0 {
		t.Fatalf("expected empty application management after deletion, got %#v", model)
	}
	sites, err := siteRepo.ListSites(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 0 {
		t.Fatalf("expected deleted application sites, got %#v", sites)
	}
}

func TestSiteManagementCanDisableSite(t *testing.T) {
	db, err := store.OpenDatabase("file:site-management-disable?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	repo := storeRepo.NewTargetAppRepository(db)
	siteRepo := storeRepo.NewAppSiteRepository(db)
	if err := repo.Add([]storeRepo.TargetApp{{RootDir: t.TempDir(), AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}
	apps, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if err := siteRepo.SyncSites(apps[0].ID, []storeRepo.AppSite{{PrimaryDomain: "ignore.example.com", ConfigPath: "site.conf", Https: true}}); err != nil {
		t.Fatal(err)
	}
	sites, err := siteRepo.ListSites(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	model := NewModel(repo, siteRepo, nil, testRegistry())
	model.screen = siteListScreen
	model.managedSites = sites
	model.managedApp = apps[0]
	if !strings.Contains(model.View(), "空格启用/禁用") || !strings.Contains(model.View(), "启用") {
		t.Fatalf("expected site management operation and status:\n%s", model.View())
	}
	updated, _ := model.updateSiteList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	model = updated.(Model)
	if model.managedSites[0].Enabled || !strings.Contains(model.status, "已禁用站点") {
		t.Fatalf("expected managed site to be disabled, got %#v", model)
	}
	sites, err = siteRepo.ListSites(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if sites[0].Enabled {
		t.Fatalf("expected disabled site to be persisted, got %#v", sites[0])
	}
}

func TestApplicationManagementViewShowsOperations(t *testing.T) {
	model := Model{
		screen: appManagementScreen,
		apps:   []storeRepo.TargetApp{{AppType: "nginx", RootDir: "C:\\nginx", Enabled: true}},
		width:  120,
	}
	if !strings.Contains(model.View(), "回车查看站点 · d 删除应用 · Esc 返回") {
		t.Fatalf("expected management operation prompt:\n%s", model.View())
	}
}

func TestHomeMenuSupportsLeftAndRightSelection(t *testing.T) {
	model := Model{menuCursor: 1}
	updated, _ := model.updateHome(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(Model)
	if model.menuCursor != 2 {
		t.Fatalf("expected right key to select next menu item, got %d", model.menuCursor)
	}
	updated, _ = model.updateHome(tea.KeyMsg{Type: tea.KeyLeft})
	model = updated.(Model)
	if model.menuCursor != 1 {
		t.Fatalf("expected left key to select previous menu item, got %d", model.menuCursor)
	}
}

func TestCertdSettingsMenuSavesJSONSetting(t *testing.T) {
	db, err := store.OpenDatabase("file:tui-certd-settings?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	settings := storeRepo.NewSettingsRepository(db)
	model := NewModelWithSettings(nil, nil, settings, nil, testRegistry())
	model.menuCursor = 3
	updated, _ := model.updateHome(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.screen != certdSettingsScreen {
		t.Fatalf("expected Certd settings screen, got %v", model.screen)
	}
	model.certdInputs[0].SetValue("https://certd.example.com")
	model.certdInputs[1].SetValue("key-id")
	model.certdInputs[2].SetValue("key-secret")
	model.certdInputs[3].SetValue("web-01")
	model.certdInputs[4].SetValue("20")
	updated, _ = model.updateCertdSettings(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.screen != homeScreen {
		t.Fatalf("expected return to home after save, got %v", model.screen)
	}
	value, err := settings.GetSetting(syncservice.CertdSettingKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(value, "key-secret") || !strings.Contains(value, "baseUrl") || !strings.Contains(value, "web-01") || !strings.Contains(value, "maxWaitMinutes") || !strings.Contains(value, "20") {
		t.Fatalf("unexpected persisted Certd setting: %s", value)
	}
}

func TestCertificateSyncProgressIsWrittenDuringExecution(t *testing.T) {
	progress := make(chan string, 1)
	progress <- "同步中：正在请求 example.com 证书"
	model := Model{syncing: true, syncProgressCh: progress}

	updated, next := model.Update(syncProgressTickMsg{})
	model = updated.(Model)
	if next == nil || len(model.logs) != 1 || !strings.Contains(model.logs[0], "正在请求 example.com") {
		t.Fatalf("expected live sync progress log, model=%#v", model)
	}
}
