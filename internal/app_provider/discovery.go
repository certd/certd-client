package app_provider

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoverApplications walks root and returns unique application roots found
// by a provider-specific evidence matcher.
func DiscoverApplications(root, appType string, report func(Progress), findRoot func(path string) (string, bool)) ([]App, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve scan root: %w", err)
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("stat scan root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("scan root is not a directory: %s", absoluteRoot)
	}

	foundRoots := make(map[string]struct{})
	pendingDirectories := []string{absoluteRoot}
	scannedDirectories := 0
	for len(pendingDirectories) > 0 {
		last := len(pendingDirectories) - 1
		directory := pendingDirectories[last]
		pendingDirectories = pendingDirectories[:last]
		entries, err := os.ReadDir(directory)
		if err != nil {
			return nil, fmt.Errorf("scan %s files: read directory %s: %w", appType, directory, err)
		}
		scannedDirectories++
		for _, entry := range entries {
			path := filepath.Join(directory, entry.Name())
			if entry.IsDir() {
				pendingDirectories = append(pendingDirectories, path)
				continue
			}
			candidate, ok := findRoot(path)
			if !ok {
				continue
			}
			candidate, err = filepath.Abs(filepath.Clean(candidate))
			if err != nil {
				return nil, fmt.Errorf("resolve %s root: %w", appType, err)
			}
			if !isWithin(absoluteRoot, candidate) {
				continue
			}
			foundRoots[candidate] = struct{}{}
		}
		if report != nil {
			report(Progress{
				ProviderType:         appType,
				ScannedDirectories:   scannedDirectories,
				RemainingDirectories: len(pendingDirectories),
			})
		}
	}

	roots := make([]string, 0, len(foundRoots))
	for root := range foundRoots {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	apps := make([]App, 0, len(roots))
	for _, root := range roots {
		apps = append(apps, App{RootDir: root, AppType: appType})
	}
	return apps, nil
}

func isWithin(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
