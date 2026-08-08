// Package app_provider defines common application discovery and site scanning contracts.
package app_provider

import "strings"

type App struct {
	RootDir string
	AppType string
}

type Site struct {
	PrimaryDomain  string
	SubdomainCount int
	ConfigPath     string
	Https          bool
}

type Progress struct {
	ProviderType         string
	ScannedDirectories   int
	RemainingDirectories int
}

type Provider interface {
	Type() string
	ScanApps(root string, report func(Progress)) ([]App, error)
	ScanSites(app App) ([]Site, error)
}

type Registry struct {
	providers map[string]Provider
	ordered   []Provider
}

func NewRegistry(providers ...Provider) *Registry {
	registry := &Registry{providers: make(map[string]Provider)}
	for _, provider := range providers {
		registry.Register(provider)
	}
	return registry
}

func (r *Registry) Register(provider Provider) {
	if provider == nil {
		return
	}
	typeName := strings.ToLower(provider.Type())
	if typeName == "" {
		return
	}
	if _, exists := r.providers[typeName]; !exists {
		r.ordered = append(r.ordered, provider)
	}
	r.providers[typeName] = provider
}

func (r *Registry) Find(appType string) (Provider, bool) {
	provider, ok := r.providers[strings.ToLower(appType)]
	return provider, ok
}

func (r *Registry) All() []Provider {
	return append([]Provider(nil), r.ordered...)
}
