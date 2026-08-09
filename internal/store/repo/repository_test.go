package repo_test

import (
	"runtime"
	"testing"

	"github.com/certd/certd-client/internal/store"
	"github.com/certd/certd-client/internal/store/repo"
)

func TestModelsAndRepositoriesAreExposedFromRepoPackage(t *testing.T) {
	db, err := store.OpenDatabase("file:repo-package?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}

	applications := repo.NewTargetAppRepository(db)
	if err := applications.Add([]repo.TargetApp{{RootDir: t.TempDir(), AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.NewAppSiteRepository(db).ListSites(1); err != nil {
		t.Fatal(err)
	}
	if err := repo.NewSettingsRepository(db).SaveSetting("certd", `{}`); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsRepositoryWritesLegacyTimestampColumns(t *testing.T) {
	db, err := store.OpenDatabase("file:legacy-settings-columns?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DROP TABLE settings").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY, setting TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.NewSettingsRepository(db).SaveSetting("certd", `{}`); err != nil {
		t.Fatalf("expected legacy timestamp columns to be populated: %v", err)
	}
}

func TestAppSiteSyncDeduplicatesIdentityAndPrefersHTTPS(t *testing.T) {
	db, err := store.OpenDatabase("file:app-site-deduplicate?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	applications := repo.NewTargetAppRepository(db)
	if err := applications.Add([]repo.TargetApp{{RootDir: t.TempDir(), AppType: "apache"}}); err != nil {
		t.Fatal(err)
	}
	apps, err := applications.List()
	if err != nil {
		t.Fatal(err)
	}
	sites := repo.NewAppSiteRepository(db)
	if err := sites.SyncSites(apps[0].ID, []repo.AppSite{
		{PrimaryDomain: "example.com", ConfigPath: "vhost.conf", Https: false},
		{PrimaryDomain: "example.com", ConfigPath: "vhost.conf", Https: true, CertificatePath: "fullchain.pem", PrivateKeyPath: "privkey.pem"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := sites.ListSites(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Https || got[0].CertificatePath != "fullchain.pem" || got[0].PrivateKeyPath != "privkey.pem" {
		t.Fatalf("expected one HTTPS site with certificate paths, got %#v", got)
	}
}

func TestAppSiteSyncTreatsWindowsConfigPathCaseAsSame(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows 配置路径大小写规则仅适用于 Windows")
	}
	db, err := store.OpenDatabase("file:app-site-config-case?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	applications := repo.NewTargetAppRepository(db)
	if err := applications.Add([]repo.TargetApp{{RootDir: t.TempDir(), AppType: "apache"}}); err != nil {
		t.Fatal(err)
	}
	apps, err := applications.List()
	if err != nil {
		t.Fatal(err)
	}
	sites := repo.NewAppSiteRepository(db)
	if err := sites.SyncSites(apps[0].ID, []repo.AppSite{
		{PrimaryDomain: "example.com", ConfigPath: `C:\Apache\Conf\VHOST.CONF`},
		{PrimaryDomain: "example.com", ConfigPath: `c:\apache\conf\vhost.conf`, Https: true},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := sites.ListSites(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Https {
		t.Fatalf("expected case-insensitive config path deduplication, got %#v", got)
	}
}
