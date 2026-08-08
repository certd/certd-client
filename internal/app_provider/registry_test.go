package app_provider

import "testing"

type testProvider struct {
	typeName string
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
