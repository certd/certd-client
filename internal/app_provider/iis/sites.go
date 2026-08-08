package iis

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/certd/certd-client/internal/app_provider"
)

type applicationHostConfiguration struct {
	Sites []iisSite `xml:"system.applicationHost>sites>site"`
}

type iisSite struct {
	Bindings []iisBinding `xml:"bindings>binding"`
}

type iisBinding struct {
	Protocol           string `xml:"protocol,attr"`
	BindingInformation string `xml:"bindingInformation,attr"`
}

func (provider IisProvider) scanSites(root string) ([]app_provider.Site, error) {
	configPath := filepath.Join(root, "config", "applicationHost.config")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取 IIS 配置文件 %s 失败: %w", configPath, err)
	}

	var config applicationHostConfiguration
	if err := xml.Unmarshal(content, &config); err != nil {
		return nil, fmt.Errorf("解析 IIS 配置文件 %s 失败: %w", configPath, err)
	}

	sites := make([]app_provider.Site, 0, len(config.Sites))
	for _, configuredSite := range config.Sites {
		domains, https := provider.domainsFromBindings(configuredSite.Bindings)
		if len(domains) == 0 {
			continue
		}
		sites = append(sites, app_provider.Site{
			PrimaryDomain:  domains[0],
			SubdomainCount: len(domains) - 1,
			ConfigPath:     configPath,
			Https:          https,
		})
	}
	sort.Slice(sites, func(i, j int) bool {
		return sites[i].PrimaryDomain < sites[j].PrimaryDomain
	})
	return sites, nil
}

func (provider IisProvider) domainsFromBindings(bindings []iisBinding) ([]string, bool) {
	domains := make([]string, 0, len(bindings))
	known := make(map[string]struct{}, len(bindings))
	https := false
	for _, binding := range bindings {
		protocol := strings.ToLower(strings.TrimSpace(binding.Protocol))
		if protocol != "http" && protocol != "https" {
			continue
		}
		if protocol == "https" {
			https = true
		}
		domain := provider.domainFromBindingInformation(binding.BindingInformation)
		if domain == "" {
			continue
		}
		if _, exists := known[domain]; exists {
			continue
		}
		known[domain] = struct{}{}
		domains = append(domains, domain)
	}
	return domains, https
}

func (IisProvider) domainFromBindingInformation(bindingInformation string) string {
	lastColon := strings.LastIndex(bindingInformation, ":")
	if lastColon == -1 {
		return ""
	}
	domain := strings.TrimSpace(bindingInformation[lastColon+1:])
	if domain == "" || domain == "*" {
		return ""
	}
	return domain
}
