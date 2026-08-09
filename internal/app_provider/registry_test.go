package app_provider

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type testProvider struct {
	typeName string
}

func TestIsPermissionDeniedRecognizesPermissionErrors(t *testing.T) {
	if !isPermissionDenied(&fs.PathError{Err: fs.ErrPermission}) {
		t.Fatal("expected permission error to be recognized")
	}
}

func TestIsSkippableReadDirErrorRecognizesDisappearedDirectory(t *testing.T) {
	if !isSkippableReadDirError(&fs.PathError{Err: fs.ErrNotExist}) {
		t.Fatal("expected disappeared directory error to be skipped")
	}
}

func TestDiscoverApplicationsSkipsDockerOverlay2Directory(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "docker", "overlay2", "container-layer", "sbin", "nginx"),
		filepath.Join(root, "normal", "sbin", "nginx"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	apps, err := DiscoverApplications(root, "nginx", nil, func(path string) (string, bool) {
		if filepath.Base(path) != "nginx" {
			return "", false
		}
		return filepath.Dir(filepath.Dir(path)), true
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].RootDir != filepath.Join(root, "normal") {
		t.Fatalf("expected only normal application to be discovered, got %#v", apps)
	}
}

func (p testProvider) Type() string { return p.typeName }

func (p testProvider) ScanApps(string, func(Progress)) ([]App, error) { return nil, nil }

func (p testProvider) ScanSites(App) ([]Site, error) { return nil, nil }

func TestRegistryRegistersAndFindsProviders(t *testing.T) {
	registry := NewRegistry(testProvider{typeName: "nginx"}, testProvider{typeName: "apache"})
	if len(registry.All()) != 2 {
		t.Fatalf("expected two providers, got %#v", registry.All())
	}
	provider, ok := registry.Find("apache")
	if !ok || provider.Type() != "apache" {
		t.Fatalf("expected Apache provider, got %#v, %v", provider, ok)
	}
}
