package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogViewerCommandByPlatform(t *testing.T) {
	cases := []struct {
		goos     string
		wantName string
	}{
		{goos: "windows", wantName: "notepad.exe"},
		{goos: "darwin", wantName: "open"},
		{goos: "linux", wantName: "xdg-open"},
	}
	for _, testCase := range cases {
		command := logViewerCommand(testCase.goos, filepath.Join("logs", "client.log"))
		if filepath.Base(command.Path) != testCase.wantName {
			t.Fatalf("unexpected log viewer for %s: %q", testCase.goos, command.Path)
		}
		if len(command.Args) != 2 || !strings.Contains(command.Args[1], "client.log") {
			t.Fatalf("expected log file argument for %s, got %#v", testCase.goos, command.Args)
		}
	}
}

func TestEnsureLogFileCreatesMissingLogFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "client.log")

	if err := ensureLogFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected log file to exist: %v", err)
	}
}
