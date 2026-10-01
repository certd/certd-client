package updater

import (
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		current, latest string
		want            int
	}{
		{"1.2.3", "1.2.4", -1},
		{"1.2.3", "1.3.0", -1},
		{"1.2.3", "2.0.0", -1},
		{"1.2.3", "1.2.3", 0},
		{"1.2.3", "1.2.2", 1},
	}
	for _, tc := range cases {
		got, err := CompareVersions(tc.current, tc.latest)
		if err != nil || got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, %v; want %d", tc.current, tc.latest, got, err, tc.want)
		}
	}
	if _, err := CompareVersions("1.2.x", "1.2.4"); err == nil {
		t.Fatal("expected malformed version error")
	}
}

func TestAssetName(t *testing.T) {
	cases := []struct{ os, arch, want string }{
		{"windows", "amd64", "certd-client-windows-amd64.zip"},
		{"linux", "arm64", "certd-client-linux-arm64.tar.gz"},
		{"darwin", "amd64", "certd-client-darwin-amd64.tar.gz"},
	}
	for _, tc := range cases {
		if got := AssetName(tc.os, tc.arch); got != tc.want {
			t.Errorf("AssetName(%q, %q) = %q, want %q", tc.os, tc.arch, got, tc.want)
		}
	}
}
