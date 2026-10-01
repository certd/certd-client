package clientreport

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/certd/certd-client/internal/certd"
	"github.com/certd/certd-client/internal/logging"
	storeRepo "github.com/certd/certd-client/internal/store/repo"
)

type fakeHeartbeatClient struct {
	payload certd.HeartbeatPayload
}

func (f *fakeHeartbeatClient) Heartbeat(payload certd.HeartbeatPayload) error {
	f.payload = payload
	return nil
}

type fakeSettings struct {
	values map[string]string
}

func (f *fakeSettings) GetSetting(key string) (string, error) {
	return f.values[key], nil
}

func (f *fakeSettings) SaveSetting(key, value string) error {
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[key] = value
	return nil
}

type fakeApps struct {
	apps []storeRepo.TargetApp
}

type testReporterLogger struct {
	logs []string
}

func (l *testReporterLogger) Info(format string, args ...any) {
	l.logs = append(l.logs, fmt.Sprintf(format, args...))
}

func (l *testReporterLogger) Warning(format string, args ...any) {}

func (l *testReporterLogger) Error(format string, args ...any) {
	l.logs = append(l.logs, fmt.Sprintf(format, args...))
}

var _ logging.Log = (*testReporterLogger)(nil)

func (f *fakeApps) List() ([]storeRepo.TargetApp, error) {
	return f.apps, nil
}

func TestGenerateClientIdIsUnique(t *testing.T) {
	first, err := generateClientId()
	if err != nil {
		t.Fatal(err)
	}
	second, err := generateClientId()
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || second == "" {
		t.Fatal("clientId 不能为空")
	}
	if first == second {
		t.Fatal("两次生成的 clientId 不应相同")
	}
}

func TestEnsureClientIdPersistsAndReuses(t *testing.T) {
	settings := &fakeSettings{}
	reporter := New(settings, nil, nil)

	first, err := reporter.ensureClientId()
	if err != nil {
		t.Fatal(err)
	}
	if first == "" {
		t.Fatal("clientId 不能为空")
	}
	if settings.values[clientSettingKey] == "" {
		t.Fatal("clientId 应持久化到 settings")
	}

	second, err := reporter.ensureClientId()
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("再次读取应复用同一 clientId，got %q want %q", second, first)
	}
}

func TestCollectStatsOnlyCountsEnabledApps(t *testing.T) {
	apps := &fakeApps{apps: []storeRepo.TargetApp{
		{Enabled: true, SiteCount: 3, HttpsSiteCount: 2, SyncedSiteCount: 1, FailedSiteCount: 0},
		{Enabled: false, SiteCount: 9, HttpsSiteCount: 8, SyncedSiteCount: 7, FailedSiteCount: 6},
	}}
	reporter := New(&fakeSettings{}, apps, nil)

	stats, err := reporter.collectStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.appCount != 1 {
		t.Fatalf("appCount = %d, want 1", stats.appCount)
	}
	if stats.siteCount != 3 {
		t.Fatalf("siteCount = %d, want 3", stats.siteCount)
	}
	if stats.httpsSiteCount != 2 {
		t.Fatalf("httpsSiteCount = %d, want 2", stats.httpsSiteCount)
	}
	if stats.syncedSiteCount != 1 {
		t.Fatalf("syncedSiteCount = %d, want 1", stats.syncedSiteCount)
	}
	if stats.failedSiteCount != 0 {
		t.Fatalf("failedSiteCount = %d, want 0", stats.failedSiteCount)
	}
}

func TestReportSkipsWhenCertdNotConfigured(t *testing.T) {
	settings := &fakeSettings{values: map[string]string{}}
	reporter := New(settings, &fakeApps{}, nil)

	// 未配置 Certd 接口时应静默跳过，不生成 clientId 也不上报。
	reporter.Report(context.Background())
	if settings.values[clientSettingKey] != "" {
		t.Fatal("未配置 Certd 接口时不应生成 clientId")
	}
}

func TestReportLogsBeforeSendingHeartbeat(t *testing.T) {
	setting, err := json.Marshal(map[string]string{
		"baseUrl":   "https://certd.example.com",
		"keyId":     "key-id",
		"keySecret": "key-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	settings := &fakeSettings{values: map[string]string{"certd": string(setting)}}
	client := &fakeHeartbeatClient{}
	logger := &testReporterLogger{}
	reporter := New(settings, &fakeApps{apps: []storeRepo.TargetApp{{
		Enabled: true, SiteCount: 2, HttpsSiteCount: 1,
	}}}, logger)
	reporter.newClient = func(_ certd.Config) HeartbeatClient { return client }

	reporter.Report(context.Background())

	if len(logger.logs) < 2 {
		t.Fatalf("应记录发送前和发送后的日志，got %v", logger.logs)
	}
	if !strings.Contains(logger.logs[0], "正在上报心跳") {
		t.Fatalf("第一条日志应说明正在上报心跳，got %q", logger.logs[0])
	}
	if !strings.Contains(logger.logs[0], "站点 2") || !strings.Contains(logger.logs[0], "HTTPS 1") {
		t.Fatalf("发送日志应包含站点统计，got %q", logger.logs[0])
	}
}

func TestMachineNameFallsBackToHostname(t *testing.T) {
	if got := machineName(""); got == "" {
		t.Fatal("空机器名应回退到主机名")
	}
	if got := machineName("   "); got == "" {
		t.Fatal("空白机器名应回退到主机名")
	}
	if got := machineName("my-server"); got != "my-server" {
		t.Fatalf("非空机器名不应改变，got %q", got)
	}
}
