package apache

import (
	"path/filepath"
	"strings"

	"github.com/certd/certd-client/internal/app_provider"
)

type ApacheProvider struct{}

func New() ApacheProvider { return ApacheProvider{} }

func (ApacheProvider) Type() string { return "apache" }

func (provider ApacheProvider) ScanApps(root string, report func(app_provider.Progress)) ([]app_provider.App, error) {
	return app_provider.DiscoverApplications(root, "apache", report, provider.apacheRootFromEvidence)
}

func (provider ApacheProvider) ScanSites(app app_provider.App) ([]app_provider.Site, error) {
	return provider.scanSites(app.RootDir)
}

func (ApacheProvider) apacheRootFromEvidence(path string) (string, bool) {
	base := strings.ToLower(filepath.Base(path))
	parent := filepath.Dir(path)
	parentName := strings.ToLower(filepath.Base(parent))
	switch base {
	case "httpd.conf", "apache2.conf":
		if parentName == "apache2" {
			return parent, true
		}
		if parentName != "conf" {
			return "", false
		}
		return filepath.Dir(parent), true
	case "httpd", "httpd.exe", "apachectl", "apachectl.exe", "apache2", "apache2.exe":
		if parentName != "bin" && parentName != "sbin" {
			return "", false
		}
		return filepath.Dir(parent), true
	default:
		return "", false
	}
}
