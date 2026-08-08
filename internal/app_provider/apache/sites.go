package apache

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/certd/certd-client/internal/app_provider"
)

func (provider ApacheProvider) scanSites(root string) ([]app_provider.Site, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve Apache root: %w", err)
	}
	configRoot := filepath.Join(absoluteRoot, "conf")
	if info, statErr := os.Stat(configRoot); statErr != nil || !info.IsDir() {
		configRoot = absoluteRoot
	}

	var sites []app_provider.Site
	err = filepath.WalkDir(configRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".conf") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read configuration %s: %w", path, readErr)
		}
		sites = append(sites, provider.parseVirtualHosts(path, string(content))...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan Apache site configurations: %w", err)
	}
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].ConfigPath == sites[j].ConfigPath {
			return sites[i].PrimaryDomain < sites[j].PrimaryDomain
		}
		return sites[i].ConfigPath < sites[j].ConfigPath
	})
	return sites, nil
}

type virtualHost struct {
	domains []string
	https   bool
}

func (provider ApacheProvider) parseVirtualHosts(configPath, content string) []app_provider.Site {
	var sites []app_provider.Site
	var current *virtualHost
	for _, line := range strings.Split(provider.stripComments(content), "\n") {
		line = strings.TrimSpace(line)
		lowerLine := strings.ToLower(line)
		if strings.HasPrefix(lowerLine, "</virtualhost") {
			if current != nil && len(current.domains) > 0 {
				sites = append(sites, app_provider.Site{
					PrimaryDomain:  current.domains[0],
					SubdomainCount: len(current.domains) - 1,
					ConfigPath:     configPath,
					Https:          current.https,
				})
			}
			current = nil
			continue
		}
		if strings.HasPrefix(lowerLine, "<virtualhost") {
			current = &virtualHost{https: strings.Contains(lowerLine, ":443")}
			continue
		}
		if current == nil {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "servername", "serveralias":
			current.domains = provider.appendUniqueDomains(current.domains, fields[1:])
		case "sslengine":
			current.https = strings.EqualFold(fields[1], "on")
		case "sslcertificatefile", "sslcertificatekeyfile":
			current.https = true
		}
	}
	return sites
}

func (ApacheProvider) appendUniqueDomains(domains []string, candidates []string) []string {
	known := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		known[domain] = struct{}{}
	}
	for _, domain := range candidates {
		domain = strings.TrimSpace(domain)
		if domain == "" || domain == "_" || strings.HasPrefix(domain, "$") {
			continue
		}
		if _, exists := known[domain]; exists {
			continue
		}
		known[domain] = struct{}{}
		domains = append(domains, domain)
	}
	return domains
}

func (ApacheProvider) stripComments(content string) string {
	var result strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	skippingComment := false
	for _, character := range content {
		if skippingComment {
			if character == '\n' {
				skippingComment = false
				result.WriteRune(character)
			}
			continue
		}
		if escaped {
			escaped = false
			result.WriteRune(character)
			continue
		}
		if character == '\\' && (inSingleQuote || inDoubleQuote) {
			escaped = true
			result.WriteRune(character)
			continue
		}
		switch character {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '#':
			if !inSingleQuote && !inDoubleQuote {
				skippingComment = true
				continue
			}
		}
		result.WriteRune(character)
	}
	return result.String()
}
