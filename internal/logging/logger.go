package logging

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// New creates the application logger and ensures the log directory exists.
func New(dir string) (*log.Logger, io.Closer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, "client.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return log.New(file, "", log.LstdFlags), file, nil
}
