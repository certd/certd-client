package nginx

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/certd/certd-client/internal/app_provider"
)

func (provider NginxProvider) scanSites(root, prefix string) ([]app_provider.Site, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve nginx root: %w", err)
	}
	configRoot := filepath.Join(absoluteRoot, "conf")
	if info, statErr := os.Stat(configRoot); statErr != nil || !info.IsDir() {
		configRoot = absoluteRoot
	}

	configurationPaths := make(map[string]struct{})
	err = filepath.WalkDir(configRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if app_provider.IsPermissionDenied(walkErr) {
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			return walkErr
		}
		if entry.IsDir() || !isNginxConfiguration(path, entry.Name()) {
			return nil
		}
		configurationPaths[path] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan nginx site configurations: %w", err)
	}
	mainConfig, err := provider.findConfig(root)
	if err != nil {
		return nil, err
	}
	if mainConfig != "" {
		included, includeErr := provider.includedConfigurationPaths(prefix, mainConfig)
		if includeErr != nil {
			return nil, includeErr
		}
		for _, path := range included {
			configurationPaths[path] = struct{}{}
		}
	}
	paths := make([]string, 0, len(configurationPaths))
	for path := range configurationPaths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var sites []app_provider.Site
	for _, path := range paths {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			if app_provider.IsPermissionDenied(readErr) {
				continue
			}
			return nil, fmt.Errorf("read configuration %s: %w", path, readErr)
		}
		sites = append(sites, provider.parseServerBlocks(prefix, path, string(content))...)
	}
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].ConfigPath == sites[j].ConfigPath {
			return sites[i].PrimaryDomain < sites[j].PrimaryDomain
		}
		return sites[i].ConfigPath < sites[j].ConfigPath
	})
	return sites, nil
}

var includeDirectivePattern = regexp.MustCompile(`(?m)^\s*include\s+([^;]+);`)

func (provider NginxProvider) includedConfigurationPaths(prefix, mainConfig string) ([]string, error) {
	visited := make(map[string]struct{})
	var visit func(string) error
	visit = func(configPath string) error {
		absolutePath, err := filepath.Abs(filepath.Clean(configPath))
		if err != nil {
			return fmt.Errorf("resolve included Nginx configuration %s: %w", configPath, err)
		}
		if _, found := visited[absolutePath]; found {
			return nil
		}
		visited[absolutePath] = struct{}{}
		content, err := os.ReadFile(absolutePath)
		if err != nil {
			return fmt.Errorf("read included Nginx configuration %s: %w", absolutePath, err)
		}
		for _, target := range provider.includeTargets(string(content)) {
			matches, err := provider.includeMatches(prefix, absolutePath, target)
			if err != nil {
				return err
			}
			for _, match := range matches {
				if err := visit(match); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(mainConfig); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(visited))
	for path := range visited {
		paths = append(paths, path)
	}
	return paths, nil
}

func (provider NginxProvider) includeTargets(content string) []string {
	matches := includeDirectivePattern.FindAllStringSubmatch(provider.stripComments(content), -1)
	targets := make([]string, 0, len(matches))
	for _, match := range matches {
		target := strings.Trim(strings.TrimSpace(match[1]), "\"'")
		if target != "" && !strings.Contains(target, "$") {
			targets = append(targets, target)
		}
	}
	return targets
}

func (NginxProvider) includeMatches(prefix, configPath, target string) ([]string, error) {
	pattern := filepath.FromSlash(target)
	patterns := []string{pattern}
	if !filepath.IsAbs(pattern) {
		patterns = []string{filepath.Join(prefix, pattern), filepath.Join(filepath.Dir(configPath), pattern)}
	}
	seen := make(map[string]struct{})
	for _, value := range patterns {
		matches, err := filepath.Glob(value)
		if err != nil {
			return nil, fmt.Errorf("parse Nginx include %s: %w", target, err)
		}
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("check included Nginx configuration %s: %w", match, err)
			}
			if !info.IsDir() {
				seen[match] = struct{}{}
			}
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func isNginxConfiguration(path, name string) bool {
	if strings.EqualFold(filepath.Ext(name), ".conf") || strings.EqualFold(name, "nginx.conf") {
		return true
	}
	for _, directory := range []string{"sites-enabled", "conf.d"} {
		if strings.EqualFold(filepath.Base(filepath.Dir(path)), directory) {
			return true
		}
	}
	return false
}

type serverBlock struct {
	domains         []string
	https           bool
	certificatePath string
	privateKeyPath  string
}

func (provider NginxProvider) parseServerBlocks(prefix, configPath, content string) []app_provider.Site {
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
				PrimaryDomain:   current.domains[0],
				Domains:         append([]string(nil), current.domains...),
				SubdomainCount:  len(current.domains) - 1,
				ConfigPath:      configPath,
				CertificatePath: provider.resolvePath(prefix, current.certificatePath),
				PrivateKeyPath:  provider.resolvePath(prefix, current.privateKeyPath),
				Https:           current.https,
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
		if strings.EqualFold(fields[0], "ssl_certificate") {
			server.certificatePath = fields[1]
		} else {
			server.privateKeyPath = fields[1]
		}
	}
}

func (NginxProvider) resolvePath(prefix, value string) string {
	if value == "" || strings.Contains(value, "$") {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(prefix, filepath.FromSlash(value))
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
