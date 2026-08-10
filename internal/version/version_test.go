package version

import "testing"

func TestDefaultVersionUsesSemanticVersion(t *testing.T) {
	if !IsSemanticVersion(Version) {
		t.Fatalf("default version must follow Node.js semantic versioning, got %q", Version)
	}
	if String() != Version {
		t.Fatalf("version string mismatch: got %q want %q", String(), Version)
	}
}

func TestIsSemanticVersion(t *testing.T) {
	for _, value := range []string{"0.1.0", "1.2.3", "1.2.3-rc.1", "1.2.3-beta.1+build.4"} {
		if !IsSemanticVersion(value) {
			t.Fatalf("expected valid semantic version: %q", value)
		}
	}
	for _, value := range []string{"v1.2.3", "1.2", "01.2.3", "1.2.3-", "1.2.3-01", "release"} {
		if IsSemanticVersion(value) {
			t.Fatalf("expected invalid semantic version: %q", value)
		}
	}
}
