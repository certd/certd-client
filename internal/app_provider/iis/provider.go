package iis

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/certd"
)

// IisProvider discovers IIS through appcmd and scans applicationHost.config.
type IisProvider struct {
	lookupExecutable func(string) (string, error)
	runCommand       func(string, ...string) ([]byte, error)
	systemRoot       func() string
	statPath         func(string) (os.FileInfo, error)
}

func New() IisProvider { return IisProvider{} }

func (IisProvider) Type() string { return "iis" }

func (provider IisProvider) ScanApps(_ string, report func(app_provider.Progress)) ([]app_provider.App, error) {
	appCmdPath, found, err := provider.findAppCmd()
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	runCommand := provider.runCommand
	if runCommand == nil {
		runCommand = func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).Output()
		}
	}
	// appcmd 已存在即可确认 IIS 已安装；普通账户可能没有读取站点配置的权限。
	_, _ = runCommand(appCmdPath, "list", "site")

	rootDir, err := filepath.Abs(filepath.Dir(appCmdPath))
	if err != nil {
		return nil, fmt.Errorf("解析 IIS 安装目录失败: %w", err)
	}
	if report != nil {
		report(app_provider.Progress{ProviderType: provider.Type(), ScannedDirectories: 1})
	}
	return []app_provider.App{{RootDir: rootDir, AppType: provider.Type()}}, nil
}

func (provider IisProvider) ScanSites(app app_provider.App) ([]app_provider.Site, error) {
	return provider.scanSites(app.RootDir)
}

func (provider IisProvider) DeployCertificate(site app_provider.Site, certificate certd.Certificate) error {
	if strings.TrimSpace(site.DeploymentName) == "" {
		return fmt.Errorf("IIS 站点名称为空")
	}
	if certificate.PfxBase64 == "" {
		return fmt.Errorf("Certd 未返回 PFX 证书")
	}
	pfx, err := base64.StdEncoding.DecodeString(certificate.PfxBase64)
	if err != nil {
		return fmt.Errorf("解析 PFX 证书失败: %w", err)
	}
	temporary, err := os.CreateTemp("", "certd-iis-*.pfx")
	if err != nil {
		return fmt.Errorf("创建 IIS 临时证书文件失败: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("设置 IIS 临时证书权限失败: %w", err)
	}
	if _, err := temporary.Write(pfx); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入 IIS 临时证书失败: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭 IIS 临时证书失败: %w", err)
	}
	runCommand := provider.runCommand
	if runCommand == nil {
		runCommand = func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		}
	}
	escapedPath := strings.ReplaceAll(temporaryPath, "'", "''")
	friendlyName := strings.TrimSpace(site.PrimaryDomain)
	if friendlyName == "" {
		friendlyName = "Certd Client"
	}
	if certificate.NotAfter.IsZero() {
		friendlyName += " 未知到期时间"
	} else {
		friendlyName += " " + certificate.NotAfter.UTC().Format("2006-01-02 15:04:05")
	}
	escapedFriendlyName := strings.ReplaceAll(friendlyName, "'", "''")
	importScript := fmt.Sprintf(`$path='%s'; $cert=New-Object System.Security.Cryptography.X509Certificates.X509Certificate2; $cert.Import($path, $null, [System.Security.Cryptography.X509Certificates.X509KeyStorageFlags]::MachineKeySet -bor [System.Security.Cryptography.X509Certificates.X509KeyStorageFlags]::PersistKeySet); $cert.FriendlyName='%s'; $store=New-Object System.Security.Cryptography.X509Certificates.X509Store('My','LocalMachine'); $store.Open('ReadWrite'); $store.Add($cert); $store.Close(); $cert.Thumbprint`, escapedPath, escapedFriendlyName)
	thumbprintOutput, err := runCommand("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", importScript)
	if err != nil {
		return fmt.Errorf("读取 IIS 证书指纹失败: %w", err)
	}
	thumbprint := strings.Join(strings.Fields(string(thumbprintOutput)), "")
	if thumbprint == "" {
		return fmt.Errorf("IIS 证书指纹为空")
	}
	bindScript := fmt.Sprintf(`Import-Module WebAdministration; $bindings=@(Get-WebBinding -Name '%s' -Protocol https); if ($bindings.Count -eq 0) { throw 'HTTPS 绑定不存在，请先创建 HTTPS 绑定' }; $bindings | ForEach-Object { $_.AddSslCertificate('%s','My') }`, strings.ReplaceAll(site.DeploymentName, "'", "''"), thumbprint)
	if _, err := runCommand("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", bindScript); err != nil {
		return fmt.Errorf("更新 IIS HTTPS 绑定失败: %w", err)
	}
	return nil
}

func (provider IisProvider) LocalCertificateExpiry(site app_provider.Site) (time.Time, error) {
	if strings.TrimSpace(site.DeploymentName) == "" {
		return time.Time{}, fmt.Errorf("IIS 站点名称为空")
	}
	runCommand := provider.runCommand
	if runCommand == nil {
		runCommand = func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		}
	}
	script := fmt.Sprintf(`Import-Module WebAdministration; $bindings=@(Get-WebBinding -Name '%s' -Protocol https); if ($bindings.Count -eq 0) { throw 'HTTPS 绑定不存在' }; $expiries=@(); foreach ($binding in $bindings) { $hash=$binding.CertificateHash; if ($hash -is [byte[]]) { $hash=([BitConverter]::ToString($hash)).Replace('-','') }; $hash=([string]$hash).Replace('-','').Replace(' ',''); $store=[string]$binding.CertificateStoreName; if ([string]::IsNullOrWhiteSpace($store)) { $store='My' }; $cert=Get-Item ('Cert:\LocalMachine\' + $store + '\' + $hash) -ErrorAction Stop; $expiries += [DateTimeOffset]$cert.NotAfter }; if ($expiries.Count -eq 0) { throw 'HTTPS 绑定证书不存在' }; $earliest=$expiries | Sort-Object | Select-Object -First 1; $earliest.ToUnixTimeSeconds()`, strings.ReplaceAll(site.DeploymentName, "'", "''"))
	output, err := runCommand("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return time.Time{}, commandError("读取 IIS 绑定证书有效期失败", err, output)
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("解析 IIS 证书有效期失败: %w", err)
	}
	return time.Unix(seconds, 0), nil
}

func commandError(message string, err error, output []byte) error {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return fmt.Errorf("%s: %w", message, err)
	}
	return fmt.Errorf("%s: %w: %s", message, err, detail)
}

func (provider IisProvider) Restart(_ app_provider.App) error {
	runCommand := provider.runCommand
	if runCommand == nil {
		runCommand = func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		}
	}
	if _, err := runCommand("iisreset.exe", "/restart"); err != nil {
		return fmt.Errorf("重启 IIS 失败: %w", err)
	}
	return nil
}

func (provider IisProvider) findAppCmd() (string, bool, error) {
	lookupExecutable := provider.lookupExecutable
	if lookupExecutable == nil {
		lookupExecutable = exec.LookPath
	}
	path, err := lookupExecutable("appcmd.exe")
	if err == nil {
		return path, true, nil
	}
	systemRoot := provider.systemRoot
	if systemRoot == nil {
		systemRoot = func() string { return os.Getenv("SystemRoot") }
	}
	rootDir := systemRoot()
	if rootDir == "" {
		return "", false, nil
	}
	path = filepath.Join(rootDir, "System32", "inetsrv", "appcmd.exe")
	statPath := provider.statPath
	if statPath == nil {
		statPath = os.Stat
	}
	if _, err := statPath(path); err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("检查 IIS appcmd 失败: %w", err)
	}
	return path, true, nil
}
