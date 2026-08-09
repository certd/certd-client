package tui

import "testing"

func TestCertificateSyncNotificationTitleIncludesFailureCountAndMachineName(t *testing.T) {
	if got := certificateSyncNotificationTitle("web-01", 3); got != "【Certd Client】 证书同步失败【数量：3】（web-01）" {
		t.Fatalf("unexpected notification title: %q", got)
	}
}
