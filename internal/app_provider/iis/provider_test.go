package iis

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/certd"
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
	if site.PrimaryDomain != "example.com" || site.SubdomainCount != 1 || !site.Https || site.ConfigPath != configPath || site.DeploymentName != "Example" || !reflect.DeepEqual(site.Domains, []string{"example.com", "www.example.com"}) {
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

func TestDeployCertificateImportsPfxAndUpdatesHttpsBinding(t *testing.T) {
	iisRoot := filepath.Join(t.TempDir(), "inetsrv")
	appCmdPath := filepath.Join(iisRoot, "appcmd.exe")
	commands := make([]string, 0, 3)
	provider := IisProvider{
		lookupExecutable: func(string) (string, error) { return appCmdPath, nil },
		runCommand: func(name string, args ...string) ([]byte, error) {
			commands = append(commands, name+" "+strings.Join(args, " "))
			if name == "powershell.exe" {
				return []byte("ABC123\r\n"), nil
			}
			return nil, nil
		},
	}

	err := provider.DeployCertificate(app_provider.Site{PrimaryDomain: "example.com", DeploymentName: "Example"}, certd.Certificate{PfxBase64: base64.StdEncoding.EncodeToString([]byte("pfx")), NotAfter: time.Date(2026, 8, 9, 12, 34, 56, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 || !strings.Contains(commands[0], "X509Store") || !strings.Contains(commands[0], "LocalMachine") || !strings.Contains(commands[0], "FriendlyName") || !strings.Contains(commands[0], "example.com 2026-08-09 12:34:56") || !strings.Contains(commands[1], "AddSslCertificate") || !strings.Contains(commands[1], "ForEach-Object") || !strings.Contains(commands[1], "ABC123") {
		t.Fatalf("unexpected IIS deployment commands: %#v", commands)
	}
}

func TestLocalCertificateExpiryReadsHttpsBindingCertificate(t *testing.T) {
	called := false
	provider := IisProvider{runCommand: func(name string, args ...string) ([]byte, error) {
		called = true
		if name != "powershell.exe" || !strings.Contains(strings.Join(args, " "), "Get-WebBinding") {
			t.Fatalf("unexpected certificate inspection command: %s %v", name, args)
		}
		return []byte("1800000000\r\n"), nil
	}}
	expiry, err := provider.LocalCertificateExpiry(app_provider.Site{DeploymentName: "Example"})
	if err != nil || expiry.Unix() != 1800000000 || !called {
		t.Fatalf("unexpected IIS certificate expiry: %v called=%v err=%v", expiry, called, err)
	}
}

func TestLocalCertificateExpirySupportsStringCertificateHash(t *testing.T) {
	var script string
	provider := IisProvider{runCommand: func(name string, args ...string) ([]byte, error) {
		script = strings.Join(args, " ")
		return []byte("1800000000\r\n"), nil
	}}
	if _, err := provider.LocalCertificateExpiry(app_provider.Site{DeploymentName: "Example"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "$bindings=@(Get-WebBinding") || !strings.Contains(script, "$hash=$binding.CertificateHash") || !strings.Contains(script, "-is [byte[]]") || !strings.Contains(script, "Sort-Object") {
		t.Fatalf("expected certificate hash type handling in script: %s", script)
	}
}

func TestLocalCertificateExpiryIncludesCommandOutputOnFailure(t *testing.T) {
	provider := IisProvider{runCommand: func(string, ...string) ([]byte, error) {
		return []byte("绑定不存在\r\n"), errors.New("exit status 1")
	}}
	_, err := provider.LocalCertificateExpiry(app_provider.Site{DeploymentName: "Example"})
	if err == nil || !strings.Contains(err.Error(), "绑定不存在") {
		t.Fatalf("expected command output in error, got %v", err)
	}
}

func TestRestartRunsIisReset(t *testing.T) {
	called := false
	provider := IisProvider{runCommand: func(name string, args ...string) ([]byte, error) {
		called = name == "iisreset.exe" && len(args) == 1 && strings.EqualFold(args[0], "/restart")
		return nil, nil
	}}
	if err := provider.Restart(app_provider.App{AppType: "iis"}); err != nil || !called {
		t.Fatalf("expected iisreset command, called=%v err=%v", called, err)
	}
}
