package store

import (
	"testing"
	"time"
)

func TestStoreCreatesAndListsTargetApps(t *testing.T) {
	db, err := OpenDatabase("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewTargetAppRepository(db)
	apps := []TargetApp{
		{RootDir: "/opt/nginx", AppType: "nginx", AddedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
		{RootDir: "/srv/nginx", AppType: "nginx", AddedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	if err := repo.Add(apps); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable("target_app") {
		t.Fatal("expected target_app table")
	}
	if err := repo.Add([]TargetApp{{RootDir: "/opt/nginx", AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.List()
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

func TestStoreUpdatesApplicationWithSameRootDirectory(t *testing.T) {
	db, err := OpenDatabase("file:target-app-update?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewTargetAppRepository(db)
	addedAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := repo.Add([]TargetApp{{RootDir: "/opt/nginx", AppType: "nginx", AddedAt: addedAt}}); err != nil {
		t.Fatal(err)
	}
	before, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Add([]TargetApp{{RootDir: "/opt/nginx", AppType: "nginx-new", AddedAt: addedAt.Add(24 * time.Hour)}}); err != nil {
		t.Fatal(err)
	}

	after, err := repo.List()
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

func TestStoreSyncsApplicationSites(t *testing.T) {
	db, err := OpenDatabase("file:app-site-sync?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewTargetAppRepository(db)
	if err := repo.Add([]TargetApp{{RootDir: "/opt/nginx", AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}
	apps, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SyncSites(apps[0].ID, []AppSite{
		{PrimaryDomain: "example.com", SubdomainCount: 2, ConfigPath: "/opt/nginx/conf/site.conf", Https: true},
		{PrimaryDomain: "plain.example.com", ConfigPath: "/opt/nginx/conf/plain.conf", Https: false},
	}); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable("app_site") {
		t.Fatal("expected app_site table")
	}
	if !db.Migrator().HasColumn(&AppSite{}, "https") || !db.Migrator().HasColumn(&AppSite{}, "app_id") {
		t.Fatal("expected lowercase app_site columns")
	}
	sites, err := repo.ListSites(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 2 || sites[0].PrimaryDomain != "example.com" || sites[0].SubdomainCount != 2 || !sites[0].Https {
		t.Fatalf("unexpected sites: %#v", sites)
	}
	apps, err = repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if apps[0].SiteCount != 2 || apps[0].HttpsSiteCount != 1 {
		t.Fatalf("unexpected application site counts: %#v", apps[0])
	}
}

func TestStoreDisablesApplicationsWithMissingRoots(t *testing.T) {
	db, err := OpenDatabase("file:disabled-apps?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewTargetAppRepository(db)
	presentRoot := t.TempDir()
	missingRoot := presentRoot + "-missing"
	if err := repo.Add([]TargetApp{
		{RootDir: presentRoot, AppType: "nginx"},
		{RootDir: missingRoot, AppType: "apache"},
	}); err != nil {
		t.Fatal(err)

	}
	disabled, err := repo.DisableMissingApps()
	if err != nil {
		t.Fatal(err)
	}
	if len(disabled) != 1 || disabled[0].RootDir != missingRoot || disabled[0].Enabled {
		t.Fatalf("unexpected disabled applications: %#v", disabled)
	}
	apps, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	byRoot := make(map[string]TargetApp, len(apps))
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
	repo := NewTargetAppRepository(db)
	if err := repo.Add([]TargetApp{{RootDir: t.TempDir(), AppType: "nginx"}}); err != nil {
		t.Fatal(err)
	}
	apps, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SyncSites(apps[0].ID, []AppSite{
		{PrimaryDomain: "example.com", ConfigPath: "site.conf"},
		{PrimaryDomain: "api.example.com", ConfigPath: "api.conf"},
	}); err != nil {
		t.Fatal(err)
	}

	deletedSites, err := repo.DeleteApp(apps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if deletedSites != 2 {
		t.Fatalf("expected 2 deleted sites, got %d", deletedSites)
	}
	apps, err = repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 0 {
		t.Fatalf("expected application to be deleted, got %#v", apps)
	}
	sites, err := repo.ListSites(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 0 {
		t.Fatalf("expected sites to be deleted, got %#v", sites)
	}
}
