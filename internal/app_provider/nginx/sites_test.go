package nginx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/certd/certd-client/internal/app_provider"
)

func TestScanSitesParsesDomainsAndHTTPS(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "conf", "vhost", "sites.conf")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `
server {
    listen 80;
    server_name example.com www.example.com api.example.com;
}

server {
    listen 443 ssl;
    server_name secure.example.com;
    ssl_certificate cert.pem;
}
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	sites, err := New().ScanSites(app_provider.App{RootDir: root, AppType: "nginx"})
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
		t.Fatalf("unexpected plain HTTP site: %#v", plain)
	}
	secure := byDomain["secure.example.com"]
	if secure.ConfigPath != configPath || secure.SubdomainCount != 0 || !secure.Https {
		t.Fatalf("unexpected HTTPS site: %#v", secure)
	}
}
