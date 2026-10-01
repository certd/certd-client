package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerWritesFileAndConsole(t *testing.T) {
	dir := t.TempDir()
	logger, closer, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	var console bytes.Buffer
	logger.SetConsole(&console)
	logger.Printf("心跳 %d", 1)
	if !strings.Contains(console.String(), "心跳 1") {
		t.Fatalf("console=%q", console.String())
	}
	content, err := os.ReadFile(filepath.Join(dir, DefaultFileName))
	if err != nil || !strings.Contains(string(content), "心跳 1") {
		t.Fatalf("file=%q err=%v", content, err)
	}
}

func TestLoggerWritesTUISinkAndFile(t *testing.T) {
	dir := t.TempDir()
	logger, closer, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	var messages []string
	logger.SetTUISink(func(message string) { messages = append(messages, message) })
	logger.Println("正在上报心跳")
	if len(messages) != 1 || !strings.Contains(messages[0], "正在上报心跳") {
		t.Fatalf("messages=%v", messages)
	}
	content, err := os.ReadFile(filepath.Join(dir, DefaultFileName))
	if err != nil || !strings.Contains(string(content), "正在上报心跳") {
		t.Fatalf("file=%q err=%v", content, err)
	}
}

func TestLoggerSeverityMethodsIncludeLevel(t *testing.T) {
	dir := t.TempDir()
	logger, closer, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	var console bytes.Buffer
	logger.SetConsole(&console)
	logger.Info("started %s", "run")
	logger.Warning("retry %d", 2)
	logger.Error("request failed: %s", "offline")
	if lines := strings.Count(console.String(), "\n"); lines != 3 {
		t.Fatalf("each severity log should occupy one console line, got %d: %q", lines, console.String())
	}
	for _, expected := range []string{"[INFO] started run", "[WARNING] retry 2", "[ERROR] request failed: offline"} {
		if !strings.Contains(console.String(), expected) {
			t.Fatalf("console output missing %q: %q", expected, console.String())
		}
	}
	content, err := os.ReadFile(filepath.Join(dir, DefaultFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"[INFO] started run", "[WARNING] retry 2", "[ERROR] request failed: offline"} {
		if !strings.Contains(string(content), expected) {
			t.Fatalf("file output missing %q: %q", expected, content)
		}
	}
}
