package apache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/certd/certd-client/internal/app_provider"
)

func TestApacheProviderImplementsProvider(t *testing.T) {
	var provider app_provider.Provider = ApacheProvider{}
	if provider.Type() != "apache" {
		t.Fatalf("unexpected provider type: %s", provider.Type())
	}
}

func TestScanAppsFindsInstallationRoot(t *testing.T) {
	root := t.TempDir()
	apacheRoot := filepath.Join(root, "Apache24")
	for _, path := range []string{
		filepath.Join(apacheRoot, "bin", "httpd.exe"),
		filepath.Join(apacheRoot, "conf", "httpd.conf"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	apps, err := New().ScanApps(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].RootDir != apacheRoot || apps[0].AppType != "apache" {
		t.Fatalf("unexpected Apache applications: %#v", apps)
	}
}

func TestScanSitesParsesVirtualHosts(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "conf", "extra", "httpd-vhosts.conf")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `
<VirtualHost *:80>
    ServerName example.com
    ServerAlias www.example.com api.example.com
</VirtualHost>

<VirtualHost *:443>
    ServerName secure.example.com
    SSLEngine on
</VirtualHost>
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	sites, err := New().ScanSites(app_provider.App{RootDir: root, AppType: "apache"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 2 {
		t.Fatalf("expected 2 sites, got %#v", sites)
	}
	byDomain := make(map[string]app_provider.Site, len(sites))
	for _, site := range sites {
		byDomain[site.PrimaryDomain] = site
	}
	plain := byDomain["example.com"]
	if plain.ConfigPath != configPath || plain.SubdomainCount != 2 || plain.Https {
		t.Fatalf("unexpected HTTP site: %#v", plain)
	}
	secure := byDomain["secure.example.com"]
	if secure.ConfigPath != configPath || secure.SubdomainCount != 0 || !secure.Https {
		t.Fatalf("unexpected HTTPS site: %#v", secure)
	}
}
