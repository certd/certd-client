//go:build windows

package updater

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"golang.org/x/sys/windows"
)

func processExited(pid int) (bool, error) {
	cmd := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid))
	out, err := cmd.Output()
	if err != nil {
		return true, nil
	}
	return !bytes.Contains(out, []byte(strconv.Itoa(pid))), nil
}
func configureDetached(cmd *exec.Cmd) {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}

func startUpdatedProcess(executable string, args []string) error {
	file, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	parameters, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return err
	}
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	if err = windows.ShellExecute(0, verb, file, parameters, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("ShellExecute runas 启动失败：%w", err)
	}
	return nil
}

func scheduleHelperCleanup(path string) {
	if path == "" {
		return
	}
	command := `Start-Sleep -Seconds 2; Remove-Item -LiteralPath $env:CERTD_HELPER -Force -ErrorAction SilentlyContinue`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", command)
	cmd.Env = append(os.Environ(), "CERTD_HELPER="+path)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err == nil {
		_ = cmd.Process.Release()
	}
}
