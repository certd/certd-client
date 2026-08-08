package nginx

import (
	"path/filepath"
	"strings"

	"github.com/certd/certd-client/internal/app_provider"
)

type NginxProvider struct{}

func New() NginxProvider { return NginxProvider{} }

func (NginxProvider) Type() string { return "nginx" }

func (provider NginxProvider) ScanApps(root string, report func(app_provider.Progress)) ([]app_provider.App, error) {
	return app_provider.DiscoverApplications(root, "nginx", report, provider.nginxRootFromEvidence)
}

func (provider NginxProvider) ScanSites(app app_provider.App) ([]app_provider.Site, error) {
	return provider.scanSites(app.RootDir)
}

func (NginxProvider) nginxRootFromEvidence(path string) (string, bool) {
	base := strings.ToLower(filepath.Base(path))
	parent := filepath.Dir(path)
	parentName := strings.ToLower(filepath.Base(parent))
	switch base {
	case "nginx.conf":
		if parentName == "nginx" {
			return parent, true
		}
		if parentName != "conf" {
			return "", false
		}
		return filepath.Dir(parent), true
	case "nginx", "nginx.exe":
		if parentName != "sbin" && parentName != "bin" {
			return "", false
		}
		return filepath.Dir(parent), true
	default:
		return "", false
	}
}
