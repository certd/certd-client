package schedule

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParseEmptyReturnsEnabledDefault(t *testing.T) {
	setting, err := Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if !setting.Enabled {
		t.Fatalf("expected default enabled, got %#v", setting)
	}
	if setting.Cron != "" {
		t.Fatalf("expected empty cron default, got %q", setting.Cron)
	}
}

func TestParseValidJSON(t *testing.T) {
	setting, err := Parse(`{"cron":"30 2 * * *","enabled":false}`)
	if err != nil {
		t.Fatal(err)
	}
	if setting.Cron != "30 2 * * *" || setting.Enabled {
		t.Fatalf("unexpected parsed setting: %#v", setting)
	}
}

func TestParseInvalidJSON(t *testing.T) {
	if _, err := Parse("not-json"); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	value, err := Setting{Cron: "0 3 * * *", Enabled: true}.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	if back.Cron != "0 3 * * *" || !back.Enabled {
		t.Fatalf("round trip mismatch: %#v raw=%s", back, value)
	}
	// 确认保存的是合法 JSON，便于其它模块读取。
	if !json.Valid([]byte(value)) {
		t.Fatalf("marshalled value is not valid json: %s", value)
	}
}

func TestResolveExpressionFallsBackToNow(t *testing.T) {
	now := time.Date(2026, 10, 3, 15, 42, 0, 0, time.Local)
	if got := ResolveExpression("", now); got != "42 15 * * *" {
		t.Fatalf("expected now-based default, got %q", got)
	}
	if got := ResolveExpression("   ", now); got != "42 15 * * *" {
		t.Fatalf("expected whitespace cron to fall back to now, got %q", got)
	}
	if got := ResolveExpression("30 2 * * *", now); got != "30 2 * * *" {
		t.Fatalf("expected provided cron passthrough, got %q", got)
	}
}

func TestRunExecutesFirstAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runs := 0
	var messages []string
	// 使用极少触发的表达式，确保首轮之后进入等待，再被取消。
	expression := "0 0 1 1 *"
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, expression, func() { runs++ }, func(message string) { messages = append(messages, message) })
	}()
	// 等待首次执行完成。
	deadline := time.After(2 * time.Second)
	for {
		if runs >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("first run did not execute in time")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error after cancel, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if runs != 1 {
		t.Fatalf("expected exactly one run before cancel, got %d", runs)
	}
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "定时任务启动成功") {
		t.Fatalf("expected start message, got %q", joined)
	}
	if !strings.Contains(joined, "定时同步已停止") {
		t.Fatalf("expected stopped message, got %q", joined)
	}
}

func TestRunRejectsInvalidExpression(t *testing.T) {
	err := Run(context.Background(), "bad cron", func() {}, nil)
	if err == nil || !strings.Contains(err.Error(), "Cron") {
		t.Fatalf("expected cron parse error, got %v", err)
	}
}

func TestNextRun(t *testing.T) {
	now := time.Date(2026, 10, 3, 15, 42, 0, 0, time.Local)
	if next, ok := NextRun("30 2 * * *", now); !ok || !next.Equal(time.Date(2026, 10, 4, 2, 30, 0, 0, time.Local)) {
		t.Fatalf("unexpected next run: %v ok=%v", next, ok)
	}
	if _, ok := NextRun("bad cron", now); ok {
		t.Fatal("expected invalid cron to report no next run")
	}
	if next, ok := NextRun("", now); !ok || next.IsZero() {
		t.Fatalf("expected default expression to yield a next run, got %v ok=%v", next, ok)
	}
}

func TestValidateAcceptsEmptyAndValidRejectsInvalid(t *testing.T) {
	if err := Validate(""); err != nil {
		t.Fatalf("empty cron should be valid: %v", err)
	}
	if err := Validate("30 2 * * *"); err != nil {
		t.Fatalf("valid cron should pass: %v", err)
	}
	if err := Validate("not-a-cron"); err == nil {
		t.Fatal("expected invalid cron error")
	}
}

func TestScheduleMessages(t *testing.T) {
	if got := StartSuccessMessage("30 2 * * *"); got != "定时任务启动成功：30 2 * * *" {
		t.Fatalf("unexpected start message: %q", got)
	}
	next := time.Date(2026, 8, 10, 2, 30, 0, 0, time.Local)
	if got := NextRunMessage(next); got != "下次执行时间：2026-08-10 02:30:00" {
		t.Fatalf("unexpected next message: %q", got)
	}
	if StoppedMessage != "定时同步已停止" {
		t.Fatalf("unexpected stopped message: %q", StoppedMessage)
	}
}
