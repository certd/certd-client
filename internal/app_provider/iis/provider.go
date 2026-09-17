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
		return commandError("读取 IIS 证书指纹失败", err, thumbprintOutput)
	}
	thumbprint := strings.Join(strings.Fields(app_provider.DecodeCommandOutput(thumbprintOutput)), "")
	if thumbprint == "" {
		return commandError("IIS 证书指纹为空", fmt.Errorf("PowerShell 未输出证书指纹"), thumbprintOutput)
	}
	// 逐绑定捕获异常并汇总失败原因：AddSslCertificate 的错误多数是非终止错误，
	// 直接放在管道中会让脚本以 0 退出，导致绑定未更新却报告部署成功。
	// 证书哈希必须传证书指纹的十六进制字符串（$cert.Thumbprint）：WebAdministration 绑定对象
	// 暴露的是 IIS 原生配置方法，其 certificateHash 参数是字符串（见 inetsrv\config\schema\rscaext.xml），
	// 传 GetCertHash() 得到的 byte[] 会让原生方法统一报“值不在预期的范围内”。.NET 的
	// Binding.AddSslCertificate(Byte[], String) 是 internal 方法，PowerShell 无法调用，
	// 其实现同样是先转换成十六进制字符串再调用同一个原生方法。
	// 失败记录附带绑定的 sslFlags、旧证书状态和新证书私钥状态，便于定位具体绑定。
	// 绑定残留无效证书（旧证书在证书存储中不存在或读取失败）时，先清除残留的 SSL 绑定再重新绑定：
	// 这种绑定本身已无法提供 HTTPS，清除后重试是恢复它的唯一途径。
	// 更新后必须回读绑定证书哈希并校验，避免非终止错误造成“绑定未更新却报告成功”。
	deploymentName := strings.ReplaceAll(site.DeploymentName, "'", "''")
	bindScript := fmt.Sprintf(`$ErrorActionPreference='Stop'; Import-Module WebAdministration; `+
		`$siteName='%s'; $thumbprint='%s'; $cert=Get-Item ('Cert:\LocalMachine\My\'+$thumbprint); `+
		`$certState=('新证书 ' + $cert.Thumbprint + ' 私钥=' + [string]$cert.HasPrivateKey + ' 存储=My'); `+
		`$bindings=@(Get-WebBinding -Name $siteName -Protocol https); `+
		`if ($bindings.Count -eq 0) { throw ('站点 ' + $siteName + ' 不存在 HTTPS 绑定，请先创建 HTTPS 绑定') }; `+
		`$failed=@(); `+
		`foreach ($binding in $bindings) { `+
		`$oldState=''; `+
		`try { `+
		`$oldHash=$binding.CertificateHash; `+
		`if ($oldHash -is [byte[]]) { $oldHash=([BitConverter]::ToString($oldHash)).Replace('-','') }; `+
		`$oldHash=([string]$oldHash).Replace('-','').Replace(' ',''); `+
		`$oldStore=[string]$binding.CertificateStoreName; `+
		`if ([string]::IsNullOrWhiteSpace($oldStore)) { $oldStore='My' }; `+
		`if ([string]::IsNullOrWhiteSpace($oldHash)) { $oldState='旧证书为空' } `+
		`elseif (Get-Item ('Cert:\LocalMachine\' + $oldStore + '\' + $oldHash) -ErrorAction SilentlyContinue) { $oldState=('旧证书存在 ' + $oldStore + '/' + $oldHash) } `+
		`else { $oldState=('旧证书已丢失 ' + $oldStore + '/' + $oldHash) } `+
		`} catch { $oldState=('旧证书读取失败：' + [string]$_) }; `+
		`$bindingError=''; `+
		`try { $binding.AddSslCertificate($cert.Thumbprint,'My') } catch { $bindingError=[string]$_ }; `+
		`if ($bindingError -ne '' -and ($oldState.StartsWith('旧证书已丢失') -or $oldState.StartsWith('旧证书读取失败'))) { `+
		`try { $binding.RemoveSslCertificate() } catch {}; `+
		`try { $binding.AddSslCertificate($cert.Thumbprint,'My'); $bindingError='' } catch { $bindingError=($bindingError + '；清除残留 SSL 绑定后重试仍失败：' + [string]$_) } }; `+
		`if ($bindingError -eq '') { `+
		`try { `+
		`$current=@(Get-WebBinding -Name $siteName -Protocol https) | Where-Object { $_.bindingInformation -eq $binding.bindingInformation } | Select-Object -First 1; `+
		`$currentHash=$current.CertificateHash; `+
		`if ($currentHash -is [byte[]]) { $currentHash=([BitConverter]::ToString($currentHash)).Replace('-','') }; `+
		`$currentHash=([string]$currentHash).Replace('-','').Replace(' ','').ToUpperInvariant(); `+
		`if ($currentHash -ne $cert.Thumbprint.ToUpperInvariant()) { $bindingError=('绑定证书未生效：当前=' + $currentHash + ' 期望=' + $cert.Thumbprint) } `+
		`} catch { $bindingError=('绑定证书回读失败：' + [string]$_) } }; `+
		`if ($bindingError -ne '') { $failed += ($binding.bindingInformation + ' [sslFlags=' + [string]$binding.sslFlags + ' ' + $oldState + '] => ' + $bindingError) } }; `+
		`if ($failed.Count -gt 0) { throw ('HTTPS 绑定证书更新失败：' + ($failed -join ' | ') + '；' + $certState) }`, deploymentName, thumbprint)
	bindOutput, err := runCommand("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", bindScript)
	if err != nil {
		return commandError("更新 IIS HTTPS 绑定失败", err, bindOutput)
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
	seconds, err := strconv.ParseInt(app_provider.DecodeCommandOutput(output), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("解析 IIS 证书有效期失败: %w", err)
	}
	return time.Unix(seconds, 0), nil
}

func commandError(message string, err error, output []byte) error {
	// Windows 命令输出为 GBK 编码，必须解码后再写入错误与日志，否则中文提示会变成乱码。
	detail := app_provider.DecodeCommandOutput(output)
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
	output, err := runCommand("iisreset.exe", "/restart")
	if err != nil {
		return commandError("重启 IIS 失败", err, output)
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
