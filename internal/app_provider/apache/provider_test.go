package apache

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/certd/certd-client/internal/app_provider"
	"golang.org/x/text/encoding/simplifiedchinese"
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
	SSLCertificateFile conf/ssl/secure/fullchain.pem
	SSLCertificateKeyFile conf/ssl/secure/privkey.pem
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
	if plain.ConfigPath != configPath || plain.SubdomainCount != 2 || plain.Https || !reflect.DeepEqual(plain.Domains, []string{"example.com", "www.example.com", "api.example.com"}) {
		t.Fatalf("unexpected HTTP site: %#v", plain)
	}
	secure := byDomain["secure.example.com"]
	if secure.ConfigPath != configPath || secure.SubdomainCount != 0 || !secure.Https || secure.CertificatePath != filepath.Join(root, "conf", "ssl", "secure", "fullchain.pem") || secure.PrivateKeyPath != filepath.Join(root, "conf", "ssl", "secure", "privkey.pem") {
		t.Fatalf("unexpected HTTPS site: %#v", secure)
	}
}

func TestRestartRestartsApacheFromApplicationRoot(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "httpd.exe")
	configPath := filepath.Join(root, "conf", "httpd.conf")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var commands [][]string
	provider := ApacheProvider{runCommand: func(name string, args ...string) ([]byte, error) {
		commands = append(commands, append([]string{name}, args...))
		return nil, nil
	}, serviceName: func(string, string) string { return "apache" }}

	if err := provider.Restart(app_provider.App{RootDir: root, AppType: "apache"}); err != nil {
		t.Fatal(err)
	}
	expected := [][]string{{"net.exe", "stop", "apache"}, {"net.exe", "start", "apache"}}
	if !reflect.DeepEqual(commands, expected) {
		t.Fatalf("unexpected Apache restart commands: %v", commands)
	}
}

func TestRestartRunsApacheGracefulFromApplicationRoot(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "httpd")
	configPath := filepath.Join(root, "conf", "httpd.conf")
	for _, path := range []string{executable, configPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var commandDir, commandName string
	var commandArgs []string
	provider := ApacheProvider{
		runCommandInDir: func(directory, name string, args ...string) ([]byte, error) {
			commandDir, commandName, commandArgs = directory, name, args
			return nil, nil
		},
		serviceName: func(string, string) string { return "" },
	}

	if err := provider.Restart(app_provider.App{RootDir: root, AppType: "apache"}); err != nil {
		t.Fatal(err)
	}
	if commandDir != root || commandName != executable || !reflect.DeepEqual(commandArgs, []string{"-f", configPath, "-k", "graceful"}) {
		t.Fatalf("unexpected Apache graceful command: dir=%q name=%q args=%v", commandDir, commandName, commandArgs)
	}
}

func TestRestartContinuesWhenApacheServiceIsAlreadyStopped(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 使用 GBK 解码服务停止提示")
	}
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "httpd.exe")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	message, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("没有启动 apache 服务。"))
	if err != nil {
		t.Fatal(err)
	}
	var commands [][]string
	provider := ApacheProvider{runCommand: func(name string, args ...string) ([]byte, error) {
		commands = append(commands, append([]string{name}, args...))
		if len(commands) == 1 {
			return message, errors.New("exit status 2")
		}
		return nil, nil
	}, serviceName: func(string, string) string { return "apache" }}

	if err := provider.Restart(app_provider.App{RootDir: root, AppType: "apache"}); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 || !reflect.DeepEqual(commands[1], []string{"net.exe", "start", "apache"}) {
		t.Fatalf("expected Apache start after an already stopped service, got %v", commands)
	}
}

func TestApacheRestartDecodesWindowsGbkOutput(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 命令输出使用 GBK")
	}
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "httpd.exe")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	gbkOutput, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("系统找不到指定的文件"))
	if err != nil {
		t.Fatal(err)
	}
	provider := ApacheProvider{runCommand: func(string, ...string) ([]byte, error) {
		return gbkOutput, errors.New("exit status 2")
	}, serviceName: func(string, string) string { return "" }}

	err = provider.Restart(app_provider.App{RootDir: root, AppType: "apache"})
	if err == nil || !strings.Contains(err.Error(), "系统找不到指定的文件") {
		t.Fatalf("expected decoded Apache error, got %v", err)
	}
}

func TestApacheServiceMatchesScOutputWithPaddedFieldSeparator(t *testing.T) {
	config := `
SERVICE_NAME: apache
        BINARY_PATH_NAME   : "C:\Data\Soft\BtSoft\apache\bin\httpd.exe" -k runservice
`
	if !apacheServiceMatches(config, `c:\data\soft\btsoft\apache`, `c:\data\soft\btsoft\apache\bin\httpd.exe`) {
		t.Fatal("expected Apache service path to match padded sc.exe field")
	}
}
