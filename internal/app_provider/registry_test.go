package app_provider

import (
	"io/fs"
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
