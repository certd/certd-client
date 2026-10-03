package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/certd/certd-client/internal/providers"
	"github.com/certd/certd-client/internal/schedule"
	"github.com/certd/certd-client/internal/store"
	storeRepo "github.com/certd/certd-client/internal/store/repo"
	"github.com/certd/certd-client/internal/version"
	"gorm.io/gorm"
)

func TestWriteStartupErrorPersistsFailure(t *testing.T) {
	logDir := t.TempDir()
	writeStartupError(logDir, "数据库初始化失败")

	content, err := os.ReadFile(filepath.Join(logDir, "client.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "启动失败：数据库初始化失败") {
		t.Fatalf("startup error was not written to log: %s", content)
	}
}

func TestParseStartCronFlag(t *testing.T) {
	value, err := parseStartCronFlag([]string{"--cron", "30 2 * * *"})
	if err != nil || value != "30 2 * * *" {
		t.Fatalf("unexpected cron flag parse: value=%q err=%v", value, err)
	}
	value, err = parseStartCronFlag(nil)
	if err != nil || value != "" {
		t.Fatalf("expected empty cron when not provided, value=%q err=%v", value, err)
	}
	if _, err := parseStartCronFlag([]string{"--unknown"}); err == nil {
		t.Fatal("expected error for unsupported flag")
	}
}

func TestResolveStartExpressionExplicitAndDefault(t *testing.T) {
	now := time.Date(2026, 8, 9, 13, 47, 30, 0, time.Local)
	if got, disabled, err := resolveStartExpression("30 2 * * *", nil, now); err != nil || disabled || got != "30 2 * * *" {
		t.Fatalf("explicit cron => (%q,%v,%v)", got, disabled, err)
	}
	if got, disabled, err := resolveStartExpression("", nil, now); err != nil || disabled || got != "47 13 * * *" {
		t.Fatalf("default cron => (%q,%v,%v)", got, disabled, err)
	}
}

func TestResolveStartExpressionReadsSettings(t *testing.T) {
	settings := storeRepo.NewSettingsRepository(openTestDB(t))
	enabled, err := schedule.Setting{Cron: "15 3 * * *", Enabled: true}.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.SaveSetting(schedule.SettingKey, enabled); err != nil {
		t.Fatal(err)
	}
	if got, disabled, err := resolveStartExpression("", settings, time.Now()); err != nil || disabled || got != "15 3 * * *" {
		t.Fatalf("stored cron => (%q,%v,%v)", got, disabled, err)
	}
	disabledContent, err := schedule.Setting{Cron: "15 3 * * *", Enabled: false}.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.SaveSetting(schedule.SettingKey, disabledContent); err != nil {
		t.Fatal(err)
	}
	if _, disabled, err := resolveStartExpression("", settings, time.Now()); err != nil || !disabled {
		t.Fatalf("expected disabled, got (disabled=%v err=%v)", disabled, err)
	}
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.OpenDatabase("file:resolve-start-settings?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSyncSummaryMessage(t *testing.T) {
	if got := syncSummaryMessage(2, 3, 1); got != "执行总结：成功 2，跳过 3，失败 1" {
		t.Fatalf("unexpected summary message: %q", got)
	}
}

func TestVersionMessage(t *testing.T) {
	if got, want := versionMessage(), "certd-client "+version.String(); got != want {
		t.Fatalf("unexpected version message: got %q want %q", got, want)
	}
}

func TestTimedServiceElevationNeeded(t *testing.T) {
	cases := []struct {
		goos   string
		isRoot bool
		want   bool
	}{
		{"linux", false, true},
		{"linux", true, false},
		{"windows", false, false},
		{"windows", true, false},
		{"darwin", false, false},
	}
	for _, c := range cases {
		if got := timedServiceElevationNeeded(c.goos, c.isRoot); got != c.want {
			t.Fatalf("timedServiceElevationNeeded(%q,%v)=%v want %v", c.goos, c.isRoot, got, c.want)
		}
	}
}

func TestServiceElevationArgs(t *testing.T) {
	got := serviceElevationArgs("/usr/local/bin/certd-client")
	want := []string{"/usr/local/bin/certd-client", "service", "ensure"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("serviceElevationArgs=%#v want %#v", got, want)
	}
}

func TestEnableTimedScheduleEnablesAndPreservesCron(t *testing.T) {
	settings := storeRepo.NewSettingsRepository(openTestDB(t))
	content, err := schedule.Setting{Cron: "15 3 * * *", Enabled: false}.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.SaveSetting(schedule.SettingKey, content); err != nil {
		t.Fatal(err)
	}
	if err := enableTimedSchedule(settings, func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local) }); err != nil {
		t.Fatal(err)
	}
	raw, err := settings.GetSetting(schedule.SettingKey)
	if err != nil {
		t.Fatal(err)
	}
	got, err := schedule.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Cron != "15 3 * * *" {
		t.Fatalf("期望启用并保留原 Cron，got %+v", got)
	}
}

func TestEnableTimedScheduleFillsDefaultCronWhenEmpty(t *testing.T) {
	settings := storeRepo.NewSettingsRepository(openTestDB(t))
	if err := settings.SaveSetting(schedule.SettingKey, ""); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 9, 13, 47, 30, 0, time.Local)
	if err := enableTimedSchedule(settings, func() time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	raw, err := settings.GetSetting(schedule.SettingKey)
	if err != nil {
		t.Fatal(err)
	}
	got, err := schedule.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Cron != "47 13 * * *" {
		t.Fatalf("期望默认按启动时刻生成 Cron 47 13 * * *，got %+v", got)
	}
}

func TestRegisteredProvidersExcludeIISOutsideWindows(t *testing.T) {
	if _, found := providers.Registered("linux").Find("iis"); found {
		t.Fatal("IIS provider must not be registered on Linux")
	}
	if _, found := providers.Registered("windows").Find("iis"); !found {
		t.Fatal("IIS provider must be registered on Windows")
	}
}

func TestOpenLogFileCreatesFile(t *testing.T) {
	logDir := t.TempDir()
	file, err := openLogFile(logDir, "client.log")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(logDir, "client.log"))
	if err != nil {
		t.Fatalf("log file not created: %v", err)
	}
	if len(content) != 0 {
		t.Fatalf("expected empty log file on first open, got %q", content)
	}
}
