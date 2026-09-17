package tui

import (
	"strings"
	"testing"
)

// 通知标题由 syncservice 统一生成：同步失败数量、本机名称和客户端来源标记。
func TestCertificateSyncNotificationTitleIncludesFailureCountAndMachineName(t *testing.T) {
	if got := certificateSyncNotificationTitle("web-01", 3); got != "证书同步失败【数量：3】（web-01） 【来自CertdClient】" {
		t.Fatalf("unexpected notification title: %q", got)
	}
}

// 未填写本机名称时使用主机名和 IPv4，标题仍保留来源标记。
func TestCertificateSyncNotificationTitleFallsBackToHostName(t *testing.T) {
	title := certificateSyncNotificationTitle("", 1)
	if !strings.Contains(title, "证书同步失败【数量：1】") || !strings.Contains(title, "【来自CertdClient】") || strings.Contains(title, "（）") {
		t.Fatalf("unexpected notification title: %q", title)
	}
}
