package nginx

import (
	"os"
	"path/filepath"
	"reflect"
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
    ssl_certificate ./ssl/cert.pem;
	ssl_certificate_key ./ssl/key.pem;
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
	if plain.ConfigPath != configPath || plain.SubdomainCount != 2 || plain.Https || !reflect.DeepEqual(plain.Domains, []string{"example.com", "www.example.com", "api.example.com"}) {
		t.Fatalf("unexpected plain HTTP site: %#v", plain)
	}
	secure := byDomain["secure.example.com"]
	if secure.ConfigPath != configPath || secure.SubdomainCount != 0 || !secure.Https || secure.CertificatePath != filepath.Join(root, "ssl", "cert.pem") || secure.PrivateKeyPath != filepath.Join(root, "ssl", "key.pem") {
		t.Fatalf("unexpected HTTPS site: %#v", secure)
	}
}

func TestScanSitesFollowsIncludesFromMainConfiguration(t *testing.T) {
	root := t.TempDir()
	mainConfig := filepath.Join(root, "conf", "nginx.conf")
	includedConfig := filepath.Join(root, "panel", "vhost", "nginx", "included.conf")
	for _, path := range []string{mainConfig, includedConfig} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	includePattern := filepath.ToSlash(filepath.Join(root, "panel", "vhost", "nginx", "*.conf"))
	if err := os.WriteFile(mainConfig, []byte("http {\n    include "+includePattern+";\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(includedConfig, []byte(`server {
    listen 443 ssl;
    server_name included.example.com;
    ssl_certificate /etc/nginx/cert.pem;
    ssl_certificate_key /etc/nginx/key.pem;
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	sites, err := New().ScanSites(app_provider.App{RootDir: root, AppType: "nginx"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].PrimaryDomain != "included.example.com" || sites[0].ConfigPath != includedConfig || !sites[0].Https {
		t.Fatalf("expected included virtual host to be scanned, got %#v", sites)
	}
}
