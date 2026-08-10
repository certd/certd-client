package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/certd/certd-client/internal/version"
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

func TestParseStartSchedule(t *testing.T) {
	now := time.Date(2026, 8, 9, 13, 47, 30, 0, time.Local)
	tests := []struct {
		name     string
		args     []string
		wantCron string
		wantNext time.Time
		wantErr  bool
	}{
		{
			name:     "custom cron",
			args:     []string{"--cron", "30 2 * * *"},
			wantCron: "30 2 * * *",
			wantNext: time.Date(2026, 8, 10, 2, 30, 0, 0, time.Local),
		},
		{
			name:     "defaults to launch time every day",
			wantCron: "47 13 * * *",
			wantNext: time.Date(2026, 8, 10, 13, 47, 0, 0, time.Local),
		},
		{
			name:    "invalid cron",
			args:    []string{"--cron", "not-a-cron"},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schedule, expression, err := parseStartSchedule(test.args, now)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected cron parsing error")
				}
				return
			}
			if err != nil || expression != test.wantCron {
				t.Fatalf("unexpected parsed cron: expression=%q err=%v", expression, err)
			}
			if next := schedule.Next(now); !next.Equal(test.wantNext) {
				t.Fatalf("unexpected next execution: got %v want %v", next, test.wantNext)
			}
		})
	}
}

func TestStartScheduleMessages(t *testing.T) {
	next := time.Date(2026, 8, 10, 2, 30, 0, 0, time.Local)
	if got := startSuccessMessage("30 2 * * *"); got != "定时任务启动成功：30 2 * * *" {
		t.Fatalf("unexpected start message: %q", got)
	}
	if got := nextExecutionMessage(next); got != "下次执行时间：2026-08-10 02:30:00" {
		t.Fatalf("unexpected next execution message: %q", got)
	}
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

func TestNewConsoleAndLogOutputWritesBothDestinations(t *testing.T) {
	logger := &capturingLogger{}
	var console []string
	output := newConsoleAndLogOutput(logger, func(values ...any) {
		console = append(console, fmt.Sprint(values...))
	})

	output("定时任务启动成功")

	if len(logger.lines) != 1 || logger.lines[0] != "定时任务启动成功" {
		t.Fatalf("expected logger output, got %#v", logger.lines)
	}
	if len(console) != 1 || console[0] != "定时任务启动成功" {
		t.Fatalf("expected console output, got %#v", console)
	}
}

func TestRegisteredProvidersExcludeIISOutsideWindows(t *testing.T) {
	if _, found := registeredProviders("linux").Find("iis"); found {
		t.Fatal("IIS provider must not be registered on Linux")
	}
	if _, found := registeredProviders("windows").Find("iis"); !found {
		t.Fatal("IIS provider must be registered on Windows")
	}
}

type capturingLogger struct {
	lines []string
}

func (logger *capturingLogger) Println(values ...any) {
	logger.lines = append(logger.lines, fmt.Sprint(values...))
}
