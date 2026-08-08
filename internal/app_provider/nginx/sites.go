package nginx

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/certd/certd-client/internal/app_provider"
)

func (provider NginxProvider) scanSites(root string) ([]app_provider.Site, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve nginx root: %w", err)
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
		sites = append(sites, provider.parseServerBlocks(path, string(content))...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan nginx site configurations: %w", err)
	}
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].ConfigPath == sites[j].ConfigPath {
			return sites[i].PrimaryDomain < sites[j].PrimaryDomain
		}
		return sites[i].ConfigPath < sites[j].ConfigPath
	})
	return sites, nil
}

type serverBlock struct {
	domains []string
	https   bool
}

func (provider NginxProvider) parseServerBlocks(configPath, content string) []app_provider.Site {
	content = provider.stripComments(content)
	var sites []app_provider.Site
	var statement strings.Builder
	depth := 0
	serverDepth := 0
	var current *serverBlock

	flushStatement := func() {
		if current == nil {
			statement.Reset()
			return
		}
		provider.parseServerDirective(current, statement.String())
		statement.Reset()
	}
	finishServer := func() {
		if current == nil {
			return
		}
		flushStatement()
		if len(current.domains) > 0 {
			sites = append(sites, app_provider.Site{
				PrimaryDomain:  current.domains[0],
				SubdomainCount: len(current.domains) - 1,
				ConfigPath:     configPath,
				Https:          current.https,
			})
		}
		current = nil
		serverDepth = 0
	}

	for _, character := range content {
		switch character {
		case '{':
			if current == nil && strings.TrimSpace(statement.String()) == "server" {
				current = &serverBlock{}
				serverDepth = depth + 1
			}
			statement.Reset()
			depth++
		case ';':
			flushStatement()
		case '}':
			if current != nil && depth == serverDepth {
				finishServer()
			}
			statement.Reset()
			if depth > 0 {
				depth--
			}
		default:
			statement.WriteRune(character)
		}
	}
	return sites
}

func (provider NginxProvider) parseServerDirective(server *serverBlock, statement string) {
	fields := strings.Fields(statement)
	if len(fields) < 2 {
		return
	}
	switch strings.ToLower(fields[0]) {
	case "server_name":
		server.domains = provider.appendUniqueDomains(server.domains, fields[1:])
	case "listen":
		for _, value := range fields[1:] {
			value = strings.ToLower(value)
			if value == "ssl" || value == "443" || strings.HasSuffix(value, ":443") {
				server.https = true
				return
			}
		}
	case "ssl_certificate", "ssl_certificate_key":
		server.https = true
	}
}

func (NginxProvider) appendUniqueDomains(domains []string, candidates []string) []string {
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

func (NginxProvider) stripComments(content string) string {
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
