package nginx

import (
	"os"
	"path/filepath"
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
