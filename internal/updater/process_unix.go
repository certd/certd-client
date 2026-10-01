//go:build !windows

package updater

import (
	"os"
	"os/exec"
)

func processExited(pid int) (bool, error) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return true, nil
	}
	err = p.Signal(os.Signal(nil))
	return err != nil, nil
}
func configureDetached(cmd *exec.Cmd) {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}

func startUpdatedProcess(executable string, args []string) error {
	cmd := exec.Command(executable, args...)
	configureDetached(cmd)
	return cmd.Start()
}

func scheduleHelperCleanup(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}
