// Package version provides the client build version.
package version

import (
	"regexp"
	"strings"
)

// Version is updated by the release script and may be overridden at build time.
var Version = "0.2.0"

var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

func String() string {
	return Version
}

func IsSemanticVersion(value string) bool {
	parts := semanticVersionPattern.FindStringSubmatch(value)
	if parts == nil {
		return false
	}
	for _, identifier := range strings.Split(parts[4], ".") {
		if len(identifier) > 1 && identifier[0] == '0' && isDigits(identifier) {
			return false
		}
	}
	return true
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
