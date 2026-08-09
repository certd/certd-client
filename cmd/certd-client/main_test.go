package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
