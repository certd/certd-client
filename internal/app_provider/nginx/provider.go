package nginx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/certd"
)

type NginxProvider struct {
	lookupExecutable    func(string) (string, error)
	runCommand          func(string, ...string) ([]byte, error)
	runCommandInDir     func(string, string, ...string) ([]byte, error)
	statPath            func(string) (os.FileInfo, error)
	lookupProcessPrefix func(string) string
}

func New() NginxProvider { return NginxProvider{} }

func (NginxProvider) Type() string { return "nginx" }

func (provider NginxProvider) ScanApps(root string, report func(app_provider.Progress)) ([]app_provider.App, error) {
	return app_provider.DiscoverApplications(root, "nginx", report, provider.nginxRootFromEvidence)
}

func (provider NginxProvider) ScanSites(app app_provider.App) ([]app_provider.Site, error) {
	prefix := app.RootDir
	if executable, err := provider.findExecutable(app.RootDir); err == nil {
		prefix = provider.effectivePrefix(app.RootDir, executable)
	}
	return provider.scanSites(app.RootDir, prefix)
}

func (NginxProvider) DeployCertificate(site app_provider.Site, certificate certd.Certificate) error {
	return app_provider.WriteCertificateFiles(site, certificate)
}

func (provider NginxProvider) Restart(app app_provider.App) error {
	executable, err := provider.findExecutable(app.RootDir)
	if err != nil {
		return err
	}
	prefix := provider.effectivePrefix(app.RootDir, executable)
	configPath, err := provider.findConfig(app.RootDir)
	if err != nil {
		return err
	}
	runCommandInDir := provider.runCommandInDir
	if runCommandInDir == nil && provider.runCommand != nil {
		runCommandInDir = func(_ string, name string, args ...string) ([]byte, error) {
			return provider.runCommand(name, args...)
		}
	}
	if runCommandInDir == nil {
		runCommandInDir = func(directory, name string, args ...string) ([]byte, error) {
			command := exec.Command(name, args...)
			command.Dir = directory
			return command.CombinedOutput()
		}
	}
	args := []string{"-p", prefix}
	if configPath != "" {
		args = append(args, "-c", provider.configArgument(prefix, configPath))
	}
	args = append(args, "-s", "reload")
	output, err := runCommandInDir(prefix, executable, args...)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return fmt.Errorf("重载 Nginx 失败: %w", err)
		}
		return fmt.Errorf("重载 Nginx 失败: %w: %s", err, detail)
	}
	return nil
}

func (NginxProvider) configArgument(prefix, configPath string) string {
	relativePath, err := filepath.Rel(prefix, configPath)
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return configPath
	}
	return filepath.ToSlash(relativePath)
}

func (provider NginxProvider) effectivePrefix(root, executable string) string {
	lookupPrefix := provider.lookupProcessPrefix
	if lookupPrefix == nil {
		lookupPrefix = provider.runningProcessPrefix
	}
	if prefix := lookupPrefix(executable); prefix != "" && filepath.IsAbs(prefix) {
		return filepath.Clean(prefix)
	}
	return root
}

func (NginxProvider) runningProcessPrefix(executable string) string {
	if runtime.GOOS != "windows" {
		return ""
	}
	quotedExecutable := "'" + strings.ReplaceAll(executable, "'", "''") + "'"
	script := "$target = " + quotedExecutable + "; Get-CimInstance Win32_Process -Filter \"Name='nginx.exe'\" | Where-Object { $_.ExecutablePath -and [string]::Equals($_.ExecutablePath, $target, [System.StringComparison]::OrdinalIgnoreCase) } | Select-Object -First 1 -ExpandProperty CommandLine"
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return ""
	}
	return NginxProvider{}.prefixFromCommandLine(string(output))
}

func (NginxProvider) prefixFromCommandLine(commandLine string) string {
	pattern := regexp.MustCompile(`(?i)(?:^|\s)-p\s+(?:"([^"]+)"|(\S+))`)
	matches := pattern.FindStringSubmatch(commandLine)
	if len(matches) == 0 {
		return ""
	}
	prefix := matches[1]
	if prefix == "" {
		prefix = matches[2]
	}
	if !filepath.IsAbs(prefix) {
		return ""
	}
	return filepath.Clean(prefix)
}

func (provider NginxProvider) findConfig(root string) (string, error) {
	statPath := provider.statPath
	if statPath == nil {
		statPath = os.Stat
	}
	for _, relativePath := range []string{
		filepath.Join("conf", "nginx.conf"),
		"nginx.conf",
	} {
		path := filepath.Join(root, relativePath)
		info, err := statPath(path)
		if err == nil && !info.IsDir() {
			return path, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("检查 Nginx 配置文件失败: %w", err)
		}
	}
	return "", nil
}

func (provider NginxProvider) findExecutable(root string) (string, error) {
	statPath := provider.statPath
	if statPath == nil {
		statPath = os.Stat
	}
	for _, relativePath := range []string{
		filepath.Join("sbin", "nginx.exe"), filepath.Join("sbin", "nginx"),
		filepath.Join("bin", "nginx.exe"), filepath.Join("bin", "nginx"),
		"nginx.exe", "nginx",
	} {
		path := filepath.Join(root, relativePath)
		info, err := statPath(path)
		if err == nil && !info.IsDir() {
			return path, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("检查 Nginx 可执行文件失败: %w", err)
		}
	}
	lookupExecutable := provider.lookupExecutable
	if lookupExecutable == nil {
		lookupExecutable = exec.LookPath
	}
	for _, name := range []string{"nginx.exe", "nginx"} {
		if path, err := lookupExecutable(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("未找到 Nginx 可执行文件: %s", root)
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
