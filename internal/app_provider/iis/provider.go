package iis

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/certd/certd-client/internal/app_provider"
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
