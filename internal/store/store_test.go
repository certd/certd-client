package store

import (
	"runtime"
	"testing"
	"time"

	"github.com/certd/certd-client/internal/store/repo"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestStoreCreatesAndListsTargetApps(t *testing.T) {
	db, err := OpenDatabase("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	targetRepo := repo.NewTargetAppRepository(db)
	apps := []repo.TargetApp{
		{RootDir: "/opt/nginx", AppType: "nginx", AddedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
		{RootDir: "/srv/nginx", AppType: "nginx", AddedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	if err := targetRepo.Add(apps); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable("target_app") {
		t.Fatal("expected target_app table")
	}
	if err := targetRepo.Add([]repo.TargetApp{{RootDir: "/opt/nginx", AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}

	got, err := targetRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected duplicate to be ignored, got %d records", len(got))
	}
	if got[0].RootDir != "/opt/nginx" {
		t.Fatalf("expected newest record first, got %#v", got)
	}
}

func TestOpenDatabaseMigratesLegacyAppSiteColumns(t *testing.T) {
	dsn := "file:legacy-app-site?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&repo.TargetApp{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE app_site (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		primary_domain TEXT NOT NULL,
		subdomain_count INTEGER NOT NULL,
		config_path TEXT NOT NULL,
		app_id INTEGER NOT NULL,
		https NUMERIC NOT NULL,
		scanned_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO app_site (primary_domain, subdomain_count, config_path, app_id, https, scanned_at, created_at, updated_at)
		VALUES ('example.com', 0, 'site.conf', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDatabase(dsn); err != nil {
		t.Fatalf("expected legacy app_site schema to migrate: %v", err)
	}
}

func TestStoreUpdatesApplicationWithSameRootDirectory(t *testing.T) {
	db, err := OpenDatabase("file:target-app-update?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	targetRepo := repo.NewTargetAppRepository(db)
	addedAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := targetRepo.Add([]repo.TargetApp{{RootDir: "/opt/nginx", AppType: "nginx", AddedAt: addedAt}}); err != nil {
		t.Fatal(err)
	}
	before, err := targetRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if err := targetRepo.Add([]repo.TargetApp{{RootDir: "/opt/nginx", AppType: "nginx-new", AddedAt: addedAt.Add(24 * time.Hour)}}); err != nil {
		t.Fatal(err)
	}

	after, err := targetRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 {
		t.Fatalf("expected one app after update, got %#v", after)
	}
	if after[0].ID != before[0].ID || after[0].AppType != "nginx-new" || !after[0].AddedAt.Equal(addedAt) {
		t.Fatalf("expected in-place update, got before=%#v after=%#v", before[0], after[0])
	}
}

func TestStoreUpdatesWindowsApplicationPathIgnoringCase(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows 路径大小写规则仅适用于 Windows")
	}
	db, err := OpenDatabase("file:target-app-case-insensitive?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	targetRepo := repo.NewTargetAppRepository(db)
	if err := targetRepo.Add([]repo.TargetApp{{RootDir: `C:\\Nginx`, AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}
	before, err := targetRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if err := targetRepo.Add([]repo.TargetApp{{RootDir: `c:\\nginx`, AppType: "apache"}}); err != nil {
		t.Fatal(err)
	}
	after, err := targetRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].ID != before[0].ID || after[0].AppType != "apache" {
		t.Fatalf("expected case-insensitive in-place update, got before=%#v after=%#v", before, after)
	}
}

func TestStoreSyncsApplicationSites(t *testing.T) {
	db, err := OpenDatabase("file:app-site-sync?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	targetRepo := repo.NewTargetAppRepository(db)
	sitesRepository := repo.NewAppSiteRepository(db)
	if err := targetRepo.Add([]repo.TargetApp{{RootDir: "/opt/nginx", AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}
	apps, err := targetRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if err := sitesRepository.SyncSites(apps[0].ID, []repo.AppSite{
		{PrimaryDomain: "example.com", SubdomainCount: 2, ConfigPath: "/opt/nginx/conf/site.conf", Https: true, SyncStatus: "synced"},
		{PrimaryDomain: "plain.example.com", ConfigPath: "/opt/nginx/conf/plain.conf", Https: false, SyncStatus: "failed"},
	}); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable("app_site") {
		t.Fatal("expected app_site table")
	}
	if !db.Migrator().HasColumn(&repo.AppSite{}, "https") || !db.Migrator().HasColumn(&repo.AppSite{}, "app_id") || !db.Migrator().HasColumn(&repo.AppSite{}, "enabled") {
		t.Fatal("expected lowercase app_site columns")
	}
	sites, err := sitesRepository.ListSites(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 2 || sites[0].PrimaryDomain != "example.com" || sites[0].SubdomainCount != 2 || !sites[0].Https || !sites[0].Enabled {
		t.Fatalf("unexpected sites: %#v", sites)
	}
	apps, err = targetRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if apps[0].SiteCount != 2 || apps[0].HttpsSiteCount != 1 || apps[0].SyncedSiteCount != 1 || apps[0].FailedSiteCount != 1 {
		t.Fatalf("unexpected application site counts: %#v", apps[0])
	}
}

func TestStorePreservesDisabledSiteWhenRescanning(t *testing.T) {
	db, err := OpenDatabase("file:disabled-site-rescan?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	targetRepo := repo.NewTargetAppRepository(db)
	sitesRepository := repo.NewAppSiteRepository(db)
	if err := targetRepo.Add([]repo.TargetApp{{RootDir: "/opt/nginx", AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}
	apps, err := targetRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	initial := []repo.AppSite{{PrimaryDomain: "example.com", ConfigPath: "/opt/nginx/conf/site.conf", Https: true}}
	if err := sitesRepository.SyncSites(apps[0].ID, initial); err != nil {
		t.Fatal(err)
	}
	sites, err := sitesRepository.ListSites(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := sitesRepository.SetEnabled(sites[0].ID, false); err != nil {
		t.Fatal(err)
	}
	sites, err = sitesRepository.ListSites(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if sites[0].Enabled {
		t.Fatalf("expected site to be disabled before rescan, got %#v", sites[0])
	}
	apps, err = targetRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if apps[0].SiteCount != 0 || apps[0].HttpsSiteCount != 0 {
		t.Fatalf("disabled sites must not be included in application counts: %#v", apps[0])
	}
	if err := sitesRepository.SyncSites(apps[0].ID, initial); err != nil {
		t.Fatal(err)
	}
	sites, err = sitesRepository.ListSites(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].Enabled {
		t.Fatalf("expected disabled state to survive rescan, got %#v", sites)
	}
	enabledSites, err := sitesRepository.ListEnabledSites(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(enabledSites) != 0 {
		t.Fatalf("disabled site must not be returned for certificate sync: %#v", enabledSites)
	}
}

func TestStoreDisablesApplicationsWithMissingRoots(t *testing.T) {
	db, err := OpenDatabase("file:disabled-apps?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	targetRepo := repo.NewTargetAppRepository(db)
	presentRoot := t.TempDir()
	missingRoot := presentRoot + "-missing"
	if err := targetRepo.Add([]repo.TargetApp{
		{RootDir: presentRoot, AppType: "nginx"},
		{RootDir: missingRoot, AppType: "apache"},
	}); err != nil {
		t.Fatal(err)

	}
	disabled, err := targetRepo.DisableMissingApps()
	if err != nil {
		t.Fatal(err)
	}
	if len(disabled) != 1 || disabled[0].RootDir != missingRoot || disabled[0].Enabled {
		t.Fatalf("unexpected disabled applications: %#v", disabled)
	}
	apps, err := targetRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	byRoot := make(map[string]repo.TargetApp, len(apps))
	for _, app := range apps {
		byRoot[app.RootDir] = app
	}
	if !byRoot[presentRoot].Enabled || byRoot[missingRoot].Enabled {
		t.Fatalf("unexpected enabled flags: %#v", byRoot)
	}
}

func TestStoreDeletesApplicationAndItsSites(t *testing.T) {
	db, err := OpenDatabase("file:delete-app-and-sites?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	targetRepo := repo.NewTargetAppRepository(db)
	sitesRepository := repo.NewAppSiteRepository(db)
	if err := targetRepo.Add([]repo.TargetApp{{RootDir: t.TempDir(), AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}
	apps, err := targetRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if err := sitesRepository.SyncSites(apps[0].ID, []repo.AppSite{
		{PrimaryDomain: "example.com", ConfigPath: "site.conf"},
		{PrimaryDomain: "api.example.com", ConfigPath: "api.conf"},
	}); err != nil {
		t.Fatal(err)
	}

	deletedSites, err := targetRepo.DeleteApp(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if deletedSites != 2 {
		t.Fatalf("expected 2 deleted sites, got %d", deletedSites)
	}
	apps, err = targetRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 0 {
		t.Fatalf("expected application to be deleted, got %#v", apps)
	}
	sites, err := sitesRepository.ListSites(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 0 {
		t.Fatalf("expected sites to be deleted, got %#v", sites)
	}
}
