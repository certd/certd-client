package iis

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/certd"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// Windows 命令输出是 GBK 编码，测试数据必须按平台还原真实字节。
func commandOutput(t *testing.T, text string) []byte {
	t.Helper()
	if runtime.GOOS != "windows" {
		return []byte(text)
	}
	encoded, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

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
	if len(commands) != 2 || !strings.Contains(commands[0], "X509Store") || !strings.Contains(commands[0], "LocalMachine") || !strings.Contains(commands[0], "FriendlyName") || !strings.Contains(commands[0], "example.com 2026-08-09 12:34:56") || !strings.Contains(commands[1], "AddSslCertificate($cert.Thumbprint,'My')") || !strings.Contains(commands[1], "foreach ($binding in") || !strings.Contains(commands[1], "ABC123") {
		t.Fatalf("unexpected IIS deployment commands: %#v", commands)
	}
	// IIS 原生配置方法（rscaext.xml）的 certificateHash 参数是字符串，
	// 传 GetCertHash() 得到的 byte[] 会让原生方法统一报“值不在预期的范围内”。
	if strings.Contains(commands[1], "AddSslCertificate($hash") {
		t.Fatalf("证书哈希必须传十六进制字符串，不能传 byte[]: %#v", commands)
	}
}

// 绑定更新后必须回读绑定证书哈希并与目标指纹比较：
// AddSslCertificate 的失败可能是非终止错误，不回读校验会出现“绑定未更新却报告成功”。
func TestDeployCertificateBindingScriptVerifiesCertificateAfterUpdate(t *testing.T) {
	scripts := make([]string, 0, 2)
	provider := IisProvider{
		runCommand: func(name string, args ...string) ([]byte, error) {
			scripts = append(scripts, args[len(args)-1])
			if len(scripts) == 1 {
				return []byte("ABC123\r\n"), nil
			}
			return nil, nil
		},
	}

	err := provider.DeployCertificate(app_provider.Site{PrimaryDomain: "example.com", DeploymentName: "Example"}, certd.Certificate{PfxBase64: base64.StdEncoding.EncodeToString([]byte("pfx")), NotAfter: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) != 2 {
		t.Fatalf("expected two PowerShell scripts: %#v", scripts)
	}
	for _, expected := range []string{"$currentHash", "绑定证书未生效"} {
		if !strings.Contains(scripts[1], expected) {
			t.Fatalf("expected %s in binding script: %s", expected, scripts[1])
		}
	}
}

// 绑定脚本由多段字符串拼成，语法错误只会在用户机器上才暴露，交付前必须用 PowerShell 解析器校验。
func TestDeployCertificateBindingScriptIsValidPowerShell(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 需要校验 PowerShell 脚本")
	}
	powerShellPath, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skipf("未找到 powershell.exe: %v", err)
	}
	scripts := make([]string, 0, 2)
	provider := IisProvider{runCommand: func(name string, args ...string) ([]byte, error) {
		scripts = append(scripts, args[len(args)-1])
		if len(scripts) == 1 {
			return []byte("ABC123\r\n"), nil
		}
		return nil, nil
	}}

	if err := provider.DeployCertificate(app_provider.Site{PrimaryDomain: "example.com", DeploymentName: "Example"}, certd.Certificate{PfxBase64: base64.StdEncoding.EncodeToString([]byte("pfx")), NotAfter: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for index, script := range scripts {
		// PowerShell 5.1 读取无 BOM 的 .ps1 会按 ANSI 解码，中文可能破坏字符串边界，必须写入 BOM。
		scriptPath := filepath.Join(t.TempDir(), fmt.Sprintf("certd-script-%d.ps1", index))
		if err := os.WriteFile(scriptPath, append([]byte{0xEF, 0xBB, 0xBF}, script...), 0o600); err != nil {
			t.Fatal(err)
		}
		checked, err := exec.Command(powerShellPath, "-NoProfile", "-NonInteractive", "-Command", fmt.Sprintf("$errors=$null; [System.Management.Automation.Language.Parser]::ParseFile('%s',[ref]$null,[ref]$errors) | Out-Null; if ($errors.Count -gt 0) { $errors | ForEach-Object { '语法错误: ' + $_.Message } } else { 'OK' }", strings.ReplaceAll(scriptPath, "'", "''"))).CombinedOutput()
		if err != nil {
			t.Fatalf("校验 PowerShell 脚本语法失败: %v: %s", err, checked)
		}
		if output := strings.TrimSpace(app_provider.DecodeCommandOutput(checked)); output != "OK" {
			t.Fatalf("PowerShell 脚本存在语法错误: %s\n%s", output, script)
		}
	}
}

// 绑定脚本必须逐个绑定捕获异常并汇总，否则 PowerShell 的非终止错误会让部署假装成功。
func TestDeployCertificateBindingScriptReportsPerBindingFailure(t *testing.T) {
	scripts := make([]string, 0, 2)
	provider := IisProvider{
		runCommand: func(name string, args ...string) ([]byte, error) {
			if name != "powershell.exe" {
				t.Fatalf("unexpected command: %s %v", name, args)
			}
			scripts = append(scripts, args[len(args)-1])
			if len(scripts) == 1 {
				return []byte("ABC123\r\n"), nil
			}
			return nil, nil
		},
	}

	err := provider.DeployCertificate(app_provider.Site{PrimaryDomain: "example.com", DeploymentName: "Example"}, certd.Certificate{PfxBase64: base64.StdEncoding.EncodeToString([]byte("pfx")), NotAfter: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) != 2 || !strings.Contains(scripts[1], "catch") || !strings.Contains(scripts[1], "throw") || !strings.Contains(scripts[1], "bindingInformation") {
		t.Fatalf("expected per-binding failure reporting in binding script: %#v", scripts)
	}
}

func TestDeployCertificateIncludesImportFailureOutput(t *testing.T) {
	provider := IisProvider{runCommand: func(string, ...string) ([]byte, error) {
		return commandOutput(t, "Import : 拒绝访问。\r\n"), errors.New("exit status 1")
	}}

	err := provider.DeployCertificate(app_provider.Site{PrimaryDomain: "example.com", DeploymentName: "Example"}, certd.Certificate{PfxBase64: base64.StdEncoding.EncodeToString([]byte("pfx")), NotAfter: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "读取 IIS 证书指纹失败") || !strings.Contains(err.Error(), "拒绝访问") || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("expected import output in error, got %v", err)
	}
}

func TestDeployCertificateReportsMissingThumbprint(t *testing.T) {
	provider := IisProvider{runCommand: func(string, ...string) ([]byte, error) {
		return []byte(" \r\n"), nil
	}}

	err := provider.DeployCertificate(app_provider.Site{PrimaryDomain: "example.com", DeploymentName: "Example"}, certd.Certificate{PfxBase64: base64.StdEncoding.EncodeToString([]byte("pfx")), NotAfter: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "IIS 证书指纹为空") {
		t.Fatalf("expected missing thumbprint error, got %v", err)
	}
}

func TestDeployCertificateIncludesBindingFailureOutput(t *testing.T) {
	calls := 0
	provider := IisProvider{runCommand: func(string, ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte("ABC123\r\n"), nil
		}
		return commandOutput(t, "绑定 *:443:example.com => 找不到方法 AddSslCertificate\r\n"), errors.New("exit status 1")
	}}

	err := provider.DeployCertificate(app_provider.Site{PrimaryDomain: "example.com", DeploymentName: "Example"}, certd.Certificate{PfxBase64: base64.StdEncoding.EncodeToString([]byte("pfx")), NotAfter: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "更新 IIS HTTPS 绑定失败") || !strings.Contains(err.Error(), "找不到方法 AddSslCertificate") || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("expected binding output in error, got %v", err)
	}
}

// 绑定失败时需要把绑定的 SSL 标志、旧证书状态和证书私钥状态写入日志，
// 便于区分绑定残留无效证书与证书导入失败，两者都会报“值不在预期的范围内”。
func TestDeployCertificateBindingScriptIncludesBindingDiagnostics(t *testing.T) {
	scripts := make([]string, 0, 2)
	provider := IisProvider{
		runCommand: func(name string, args ...string) ([]byte, error) {
			if name != "powershell.exe" {
				t.Fatalf("unexpected command: %s %v", name, args)
			}
			scripts = append(scripts, args[len(args)-1])
			if len(scripts) == 1 {
				return []byte("ABC123\r\n"), nil
			}
			return nil, nil
		},
	}

	err := provider.DeployCertificate(app_provider.Site{PrimaryDomain: "example.com", DeploymentName: "Example"}, certd.Certificate{PfxBase64: base64.StdEncoding.EncodeToString([]byte("pfx")), NotAfter: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) != 2 {
		t.Fatalf("expected two PowerShell scripts: %#v", scripts)
	}
	for _, expected := range []string{"sslFlags", "HasPrivateKey", "CertificateHash", "CertificateStoreName", "RemoveSslCertificate"} {
		if !strings.Contains(scripts[1], expected) {
			t.Fatalf("expected %s in binding script: %s", expected, scripts[1])
		}
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
		return commandOutput(t, "绑定不存在\r\n"), errors.New("exit status 1")
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

func TestRestartIncludesCommandOutputOnFailure(t *testing.T) {
	provider := IisProvider{runCommand: func(string, ...string) ([]byte, error) {
		return commandOutput(t, "尝试停止 IIS 服务失败，拒绝访问。\r\n"), errors.New("exit status 5")
	}}
	err := provider.Restart(app_provider.App{AppType: "iis"})
	if err == nil || !strings.Contains(err.Error(), "重启 IIS 失败") || !strings.Contains(err.Error(), "拒绝访问") || !strings.Contains(err.Error(), "exit status 5") {
		t.Fatalf("expected restart output in error, got %v", err)
	}
}

func TestDeployCertificateDecodesWindowsGbkBindingOutput(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 命令输出使用 GBK 解码")
	}
	calls := 0
	provider := IisProvider{runCommand: func(string, ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte("ABC123\r\n"), nil
		}
		return commandOutput(t, "HTTPS 绑定证书更新失败：*:5443:aaa.handfree.work => 值不在预期的范围内。\r\n"), errors.New("exit status 1")
	}}

	err := provider.DeployCertificate(app_provider.Site{PrimaryDomain: "aaa.handfree.work", DeploymentName: "aaa.handfree.work"}, certd.Certificate{PfxBase64: base64.StdEncoding.EncodeToString([]byte("pfx")), NotAfter: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "更新 IIS HTTPS 绑定失败") || !strings.Contains(err.Error(), "值不在预期的范围内") {
		t.Fatalf("expected decoded GBK binding error, got %v", err)
	}
	if strings.Contains(err.Error(), "\ufffd") {
		t.Fatalf("expected readable Chinese in binding error, got %v", err)
	}
}
