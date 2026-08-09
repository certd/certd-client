package apache

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/certd"
	"golang.org/x/text/encoding/simplifiedchinese"
)

type ApacheProvider struct {
	lookupExecutable func(string) (string, error)
	runCommand       func(string, ...string) ([]byte, error)
	statPath         func(string) (os.FileInfo, error)
	serviceName      func(string, string) string
}

func New() ApacheProvider { return ApacheProvider{} }

func (ApacheProvider) Type() string { return "apache" }

func (provider ApacheProvider) ScanApps(root string, report func(app_provider.Progress)) ([]app_provider.App, error) {
	return app_provider.DiscoverApplications(root, "apache", report, provider.apacheRootFromEvidence)
}

func (provider ApacheProvider) ScanSites(app app_provider.App) ([]app_provider.Site, error) {
	return provider.scanSites(app.RootDir)
}

func (ApacheProvider) DeployCertificate(site app_provider.Site, certificate certd.Certificate) error {
	return app_provider.WriteCertificateFiles(site, certificate)
}

func (provider ApacheProvider) Restart(app app_provider.App) error {
	executable, err := provider.findExecutable(app.RootDir)
	if err != nil {
		return err
	}
	serviceName := provider.serviceName
	if serviceName == nil {
		serviceName = provider.findServiceName
	}
	configPath, err := provider.findConfig(app.RootDir)
	if err != nil {
		return err
	}
	runCommand := provider.runCommand
	if runCommand == nil {
		runCommand = func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		}
	}
	if name := serviceName(app.RootDir, executable); name != "" {
		if output, err := runCommand("net.exe", "stop", name); err != nil {
			if !isServiceNotRunning(output) {
				return formatRestartError("停止 Apache 服务失败", output, err)
			}
		}
		if output, err := runCommand("net.exe", "start", name); err != nil {
			return formatRestartError("启动 Apache 服务失败", output, err)
		}
		return nil
	}
	args := make([]string, 0, 4)
	if configPath != "" {
		args = append(args, "-f", configPath)
	}
	args = append(args, "-k", "graceful")
	output, err := runCommand(executable, args...)
	if err != nil {
		return formatRestartError("重启 Apache 失败", output, err)
	}
	return nil
}

func formatRestartError(action string, output []byte, err error) error {
	detail := decodeCommandOutput(output)
	if detail == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, detail)
}

func isServiceNotRunning(output []byte) bool {
	detail := strings.ToLower(decodeCommandOutput(output))
	for _, phrase := range []string{"没有启动", "服务未启动", "not started", "service is not running"} {
		if strings.Contains(detail, phrase) {
			return true
		}
	}
	return false
}

// findServiceName returns the Windows service that owns this Apache executable.
// Apache defaults to Apache2.4, which is not the name used by several panels.
func (ApacheProvider) findServiceName(root, executable string) string {
	if runtime.GOOS != "windows" {
		return ""
	}
	output, err := exec.Command("sc.exe", "query", "type=", "service", "state=", "all").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		const prefix = "SERVICE_NAME:"
		if !strings.HasPrefix(strings.ToUpper(line), prefix) {
			continue
		}
		name := strings.TrimSpace(line[len(prefix):])
		if name == "" {
			continue
		}
		config, queryErr := exec.Command("sc.exe", "qc", name).Output()
		if queryErr != nil || !apacheServiceMatches(string(config), root, executable) {
			continue
		}
		return name
	}
	return ""
}

func apacheServiceMatches(config, root, executable string) bool {
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), "BINARY_PATH_NAME") {
			continue
		}
		binaryPath := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(parts[1]), "/", "\\"))
		executable = strings.ToLower(strings.ReplaceAll(executable, "/", "\\"))
		root = strings.ToLower(strings.ReplaceAll(root, "/", "\\"))
		return strings.Contains(binaryPath, executable) || strings.Contains(binaryPath, root)
	}
	return false
}

func (provider ApacheProvider) findConfig(root string) (string, error) {
	statPath := provider.statPath
	if statPath == nil {
		statPath = os.Stat
	}
	for _, relativePath := range []string{
		filepath.Join("conf", "httpd.conf"),
		filepath.Join("conf", "apache2.conf"),
		"httpd.conf",
	} {
		path := filepath.Join(root, relativePath)
		info, err := statPath(path)
		if err == nil && !info.IsDir() {
			return path, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("检查 Apache 配置文件失败: %w", err)
		}
	}
	return "", nil
}

func decodeCommandOutput(output []byte) string {
	if runtime.GOOS != "windows" {
		return strings.TrimSpace(string(output))
	}
	decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(output)
	if err != nil {
		return strings.TrimSpace(string(output))
	}
	return strings.TrimSpace(string(decoded))
}

func (provider ApacheProvider) findExecutable(root string) (string, error) {
	statPath := provider.statPath
	if statPath == nil {
		statPath = os.Stat
	}
	for _, relativePath := range []string{
		filepath.Join("bin", "httpd.exe"), filepath.Join("bin", "httpd"),
		filepath.Join("sbin", "httpd.exe"), filepath.Join("sbin", "httpd"),
		filepath.Join("bin", "apachectl.exe"), filepath.Join("bin", "apachectl"),
		"httpd.exe", "httpd",
	} {
		path := filepath.Join(root, relativePath)
		info, err := statPath(path)
		if err == nil && !info.IsDir() {
			return path, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("检查 Apache 可执行文件失败: %w", err)
		}
	}
	lookupExecutable := provider.lookupExecutable
	if lookupExecutable == nil {
		lookupExecutable = exec.LookPath
	}
	for _, name := range []string{"httpd.exe", "httpd", "apachectl.exe", "apachectl"} {
		if path, err := lookupExecutable(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("未找到 Apache 可执行文件: %s", root)
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
