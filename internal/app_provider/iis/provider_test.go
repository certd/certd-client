package iis

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/certd/certd-client/internal/app_provider"
)

func TestIisProviderImplementsProvider(t *testing.T) {
	var provider app_provider.Provider = IisProvider{}
	if provider.Type() != "iis" {
		t.Fatalf("unexpected provider type: %s", provider.Type())
	}
}

func TestScanAppsUsesAppCmdToDetectIis(t *testing.T) {
	iisRoot := filepath.Join(t.TempDir(), "inetsrv")
	appCmdPath := filepath.Join(iisRoot, "appcmd.exe")
	called := false
	provider := IisProvider{
		lookupExecutable: func(name string) (string, error) {
			if name != "appcmd.exe" {
				t.Fatalf("unexpected executable lookup: %s", name)
			}
			return appCmdPath, nil
		},
		runCommand: func(name string, args ...string) ([]byte, error) {
			called = true
			if name != appCmdPath || len(args) != 2 || args[0] != "list" || args[1] != "site" {
				t.Fatalf("unexpected command: %s %v", name, args)
			}
			return []byte("SITE \"Default Web Site\""), nil
		},
	}

	apps, err := provider.ScanApps(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected appcmd command to be called")
	}
	if len(apps) != 1 || apps[0] != (app_provider.App{RootDir: iisRoot, AppType: "iis"}) {
		t.Fatalf("unexpected IIS applications: %#v", apps)
	}
}

func TestScanAppsSkipsWhenIisIsNotInstalled(t *testing.T) {
	provider := IisProvider{
		lookupExecutable: func(string) (string, error) {
			return "", errors.New("executable file not found")
		},
		systemRoot: func() string { return "" },
	}

	apps, err := provider.ScanApps(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 0 {
		t.Fatalf("expected no IIS applications, got %#v", apps)
	}
}

func TestFindAppCmdUsesSystemInetsrvPath(t *testing.T) {
	systemRoot := t.TempDir()
	appCmdPath := filepath.Join(systemRoot, "System32", "inetsrv", "appcmd.exe")
	if err := os.MkdirAll(filepath.Dir(appCmdPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appCmdPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	provider := IisProvider{
		lookupExecutable: func(string) (string, error) {
			return "", errors.New("executable file not found")
		},
		systemRoot: func() string { return systemRoot },
	}

	path, found, err := provider.findAppCmd()
	if err != nil || !found || path != appCmdPath {
		t.Fatalf("unexpected IIS command lookup: path=%q found=%v err=%v", path, found, err)
	}
}

func TestFindAppCmdReturnsFileCheckError(t *testing.T) {
	provider := IisProvider{
		lookupExecutable: func(string) (string, error) {
			return "", errors.New("executable file not found")
		},
		systemRoot: func() string { return "C:\\Windows" },
		statPath: func(string) (os.FileInfo, error) {
			return nil, errors.New("access denied")
		},
	}

	_, _, err := provider.findAppCmd()
	if err == nil || err.Error() != "检查 IIS appcmd 失败: access denied" {
		t.Fatalf("unexpected file check error: %v", err)
	}
}

func TestScanAppsKeepsIisWhenAppCmdCannotReadSites(t *testing.T) {
	iisRoot := filepath.Join(t.TempDir(), "inetsrv")
	provider := IisProvider{
		lookupExecutable: func(string) (string, error) {
			return filepath.Join(iisRoot, "appcmd.exe"), nil
		},
		runCommand: func(string, ...string) ([]byte, error) {
			return nil, errors.New("exit status 5")
		},
	}

	apps, err := provider.ScanApps(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0] != (app_provider.App{RootDir: iisRoot, AppType: "iis"}) {
		t.Fatalf("unexpected IIS applications: %#v", apps)
	}
}

func TestScanAppsReportsCompletedProgress(t *testing.T) {
	provider := IisProvider{
		lookupExecutable: func(string) (string, error) {
			return filepath.Join(t.TempDir(), "inetsrv", "appcmd.exe"), nil
		},
		runCommand: func(string, ...string) ([]byte, error) { return nil, nil },
	}
	var progress app_provider.Progress

	_, err := provider.ScanApps(t.TempDir(), func(update app_provider.Progress) {
		progress = update
	})
	if err != nil {
		t.Fatal(err)
	}
	if progress.ProviderType != "iis" || progress.ScannedDirectories != 1 || progress.RemainingDirectories != 0 {
		t.Fatalf("unexpected IIS progress: %#v", progress)
	}
}

func TestScanSitesParsesApplicationHostConfig(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config", "applicationHost.config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `<?xml version="1.0" encoding="UTF-8"?>
<configuration>
  <system.applicationHost>
    <sites>
      <site name="Example" id="1">
        <bindings>
          <binding protocol="http" bindingInformation="*:80:example.com" />
          <binding protocol="https" bindingInformation="*:443:www.example.com" />
          <binding protocol="https" bindingInformation="*:443:example.com" />
        </bindings>
      </site>
      <site name="NoHostHeader" id="2">
        <bindings><binding protocol="http" bindingInformation="*:80:" /></bindings>
      </site>
    </sites>
  </system.applicationHost>
</configuration>`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	sites, err := New().ScanSites(app_provider.App{RootDir: root, AppType: "iis"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 {
		t.Fatalf("expected one IIS site, got %#v", sites)
	}
	site := sites[0]
	if site.PrimaryDomain != "example.com" || site.SubdomainCount != 1 || !site.Https || site.ConfigPath != configPath {
		t.Fatalf("unexpected IIS site: %#v", site)
	}
}

func TestScanSitesRejectsInvalidConfiguration(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config", "applicationHost.config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("<configuration>"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := New().ScanSites(app_provider.App{RootDir: root, AppType: "iis"})
	if err == nil {
		t.Fatal("expected invalid IIS configuration error")
	}
}

func TestDomainsFromBindingsSkipsUnsupportedAndInvalidDomains(t *testing.T) {
	domains, https := New().domainsFromBindings([]iisBinding{
		{Protocol: "net.tcp", BindingInformation: "*:808:*"},
		{Protocol: "https", BindingInformation: "invalid"},
		{Protocol: "http", BindingInformation: "*:80:*"},
	})
	if len(domains) != 0 || !https {
		t.Fatalf("unexpected parsed bindings: domains=%#v https=%v", domains, https)
	}
}
