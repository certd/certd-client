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
	tea "github.com/charmbracelet/bubbletea"
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
	for i := 1; i <= 12; i++ {
		model.logs = append(model.logs, fmt.Sprintf("log-%02d", i))
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(Model)
	if model.logScroll != logPageSize {
		t.Fatalf("expected page up to move by %d, got %d", logPageSize, model.logScroll)
	}
	if !strings.Contains(model.View(), "log-07") || strings.Contains(model.View(), "log-12") {
		t.Fatal("page up should show an older log page")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(Model)
	if model.logScroll != 0 {
		t.Fatalf("expected page down to return to bottom, got %d", model.logScroll)
	}
	if !strings.Contains(model.View(), "log-12") {
		t.Fatal("page down should show the latest log page")
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
	model := NewModel(nil, nil, testRegistry())
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
	repo := store.NewTargetAppRepository(db)
	missingRoot := filepath.Join(t.TempDir(), "missing")
	if err := repo.Add([]store.TargetApp{{RootDir: missingRoot, AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}
	model := NewModel(repo, nil, testRegistry())
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
	if len(menuItems) != 3 || menuItems[1] != "扫描站点" {
		t.Fatalf("expected site scan menu item, got %#v", menuItems)
	}
}

func TestApplicationScanUsesAllRegisteredProviders(t *testing.T) {
	registry := app_provider.NewRegistry(
		staticProvider{typeName: "nginx", apps: []app_provider.App{{RootDir: "C:\\nginx", AppType: "nginx"}}},
		staticProvider{typeName: "apache", apps: []app_provider.App{{RootDir: "C:\\apache", AppType: "apache"}}},
	)
	model := NewModel(nil, nil, registry)
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
	model := Model{apps: []store.TargetApp{{AppType: "nginx", RootDir: "C:\\nginx", Enabled: true, SiteCount: 2, HttpsSiteCount: 1}}}
	view := model.View()
	for _, text := range []string{"类型", "安装目录", "站点数", "HTTPS站点数", "nginx", "C:\\nginx", "2", "1"} {
		if !strings.Contains(view, text) {
			t.Fatalf("expected %q in registered apps view:\n%s", text, view)
		}
	}
	if strings.Contains(view, "0001-01-01") {
		t.Fatalf("registered apps view should not render add time:\n%s", view)
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
	repo := store.NewTargetAppRepository(db)
	if err := repo.Add([]store.TargetApp{{RootDir: root, AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}
	apps, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}

	model := NewModel(repo, nil, testRegistry())
	updated, _ := model.Update(model.scanSites(apps)())
	model = updated.(Model)
	sites, err := repo.ListSites(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if model.siteScanning || len(sites) != 1 || sites[0].PrimaryDomain != "example.com" || sites[0].SubdomainCount != 1 || !sites[0].Https {
		t.Fatalf("unexpected site scan result: model=%#v sites=%#v", model, sites)
	}
}

func TestApplicationManagementViewsSitesAndDeletesApplication(t *testing.T) {
	db, err := store.OpenDatabase("file:application-management?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	repo := store.NewTargetAppRepository(db)
	if err := repo.Add([]store.TargetApp{{RootDir: t.TempDir(), AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}
	apps, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SyncSites(apps[0].ID, []store.AppSite{{PrimaryDomain: "example.com", ConfigPath: "site.conf"}}); err != nil {
		t.Fatal(err)
	}

	model := NewModel(repo, nil, testRegistry())
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
	sites, err := repo.ListSites(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 0 {
		t.Fatalf("expected deleted application sites, got %#v", sites)
	}
}

func TestApplicationManagementViewShowsOperations(t *testing.T) {
	model := Model{
		screen: appManagementScreen,
		apps:   []store.TargetApp{{AppType: "nginx", RootDir: "C:\\nginx", Enabled: true}},
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
