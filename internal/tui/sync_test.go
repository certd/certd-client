package tui

import (
	"strings"
	"testing"

	storeRepo "github.com/certd/certd-client/internal/store/repo"
	"github.com/certd/certd-client/internal/syncservice"
)

func TestSyncLabelsIncludeApplicationTypeAndID(t *testing.T) {
	app := storeRepo.TargetApp{ID: 7, AppType: "iis", RootDir: `C:\\Windows\\System32\\inetsrv`}
	label := syncservice.AppLabel(app)
	if !strings.Contains(label, "iis") || strings.Contains(label, "7") {
		t.Fatalf("expected sync app label to include type without application ID, got %q", label)
	}

	siteLabel := syncservice.SiteLabel(app, storeRepo.AppSite{ID: 42, PrimaryDomain: "example.com"})
	if !strings.Contains(siteLabel, "iis") || !strings.Contains(siteLabel, "42") || !strings.Contains(siteLabel, "example.com") || strings.Contains(siteLabel, "7") {
		t.Fatalf("expected sync site label to include app type, site ID and domain, got %q", siteLabel)
	}
}
