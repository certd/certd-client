package nginx

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/certd/certd-client/internal/app_provider"
)

func TestNginxProviderImplementsProvider(t *testing.T) {
	var provider app_provider.Provider = NginxProvider{}
	if provider.Type() != "nginx" {
		t.Fatalf("unexpected provider type: %s", provider.Type())
	}
}

func TestScanAppsFindsInstallationRootsAndDeduplicates(t *testing.T) {
	root := t.TempDir()
	nginxA := filepath.Join(root, "opt", "nginx")
	nginxB := filepath.Join(root, "panel", "nginx")
	nginxC := filepath.Join(root, "etc", "nginx")
	for _, path := range []string{
		filepath.Join(nginxA, "sbin", "nginx"),
		filepath.Join(nginxA, "conf", "nginx.conf"),
		filepath.Join(nginxB, "conf", "nginx.conf"),
		filepath.Join(nginxC, "nginx.conf"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "nginx.conf"), []byte("not an install"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := New().ScanApps(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 installations, got %d: %#v", len(got), got)
	}
	seen := make(map[string]bool, len(got))
	for _, item := range got {
		seen[item.RootDir] = true
	}
	for _, want := range []string{nginxA, nginxB, nginxC} {
		if !seen[want] {
			t.Fatalf("missing root %s in %#v", want, got)
		}
	}
}

func TestScanAppsRejectsMissingRoot(t *testing.T) {
	_, err := New().ScanApps(filepath.Join(t.TempDir(), "missing"), nil)
	if err == nil {
		t.Fatal("expected error for missing root")
	}
}

func TestScanAppsReportsDirectoryProgress(t *testing.T) {
	root := t.TempDir()
	progress := make([]app_provider.Progress, 0)
	_, err := New().ScanApps(root, func(update app_provider.Progress) {
		progress = append(progress, update)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) == 0 {
		t.Fatal("expected progress updates")
	}
	last := progress[len(progress)-1]
	if last.ProviderType != "nginx" || last.ScannedDirectories != 1 || last.RemainingDirectories != 0 {
		t.Fatalf("unexpected final progress: %#v", last)
	}
}

func TestRestartReloadsNginxFromApplicationRoot(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "sbin", "nginx.exe")
	configPath := filepath.Join(root, "conf", "nginx.conf")
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
	var commandName string
	var commandArgs []string
	var commandDir string
	provider := NginxProvider{runCommandInDir: func(directory, name string, args ...string) ([]byte, error) {
		commandDir = directory
		commandName = name
		commandArgs = args
		return nil, nil
	}}

	if err := provider.Restart(app_provider.App{RootDir: root, AppType: "nginx"}); err != nil {
		t.Fatal(err)
	}
	if commandDir != root || commandName != executable || len(commandArgs) != 6 || commandArgs[0] != "-p" || commandArgs[1] != root || commandArgs[2] != "-c" || commandArgs[3] != "conf/nginx.conf" || commandArgs[4] != "-s" || commandArgs[5] != "reload" {
		t.Fatalf("unexpected nginx reload command: %s %v", commandName, commandArgs)
	}
}

func TestRestartPreservesRunningNginxPrefix(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "sbin", "nginx.exe")
	configPath := filepath.Join(root, "conf", "nginx.conf")
	prefix := filepath.Join(root, "custom-prefix")
	for _, path := range []string{executable, configPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var commandArgs []string
	provider := NginxProvider{
		runCommand: func(_ string, args ...string) ([]byte, error) {
			commandArgs = args
			return nil, nil
		},
		lookupProcessPrefix: func(path string) string {
			if path != executable {
				t.Fatalf("unexpected executable lookup: %s", path)
			}
			return prefix
		},
	}

	if err := provider.Restart(app_provider.App{RootDir: root, AppType: "nginx"}); err != nil {
		t.Fatal(err)
	}
	if len(commandArgs) != 6 || commandArgs[0] != "-p" || commandArgs[1] != prefix || commandArgs[2] != "-c" || commandArgs[3] != configPath || commandArgs[4] != "-s" || commandArgs[5] != "reload" {
		t.Fatalf("unexpected nginx reload command: %v", commandArgs)
	}
}

func TestPrefixFromCommandLineUsesAbsoluteCustomPrefix(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows 进程命令行使用 Windows 路径")
	}
	prefix := New().prefixFromCommandLine(`"C:\Nginx\nginx.exe" -p "C:\Custom Nginx" -c conf/nginx.conf`)
	if prefix != `C:\Custom Nginx` {
		t.Fatalf("unexpected nginx prefix: %q", prefix)
	}
}
