package updater

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type helperPayload struct {
	Archive, Executable, Helper string
	PID                         int
	Args                        []string
}

func StartReplacement(archive, executable string, pid int, args []string) error {
	helper, err := copyHelperExecutable(executable)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(helperPayload{Archive: archive, Executable: executable, Helper: helper, PID: pid, Args: args})
	if err != nil {
		_ = os.Remove(helper)
		return err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	cmd := exec.Command(helper, "__apply-update", encoded)
	configureDetached(cmd)
	if err = cmd.Start(); err != nil {
		_ = os.Remove(helper)
		return fmt.Errorf("启动更新 helper 失败：%w", err)
	}
	return cmd.Process.Release()
}

func copyHelperExecutable(executable string) (string, error) {
	source, err := os.Open(executable)
	if err != nil {
		return "", err
	}
	defer source.Close()
	helper, err := os.CreateTemp(filepath.Dir(executable), ".certd-client-helper-*.exe")
	if err != nil {
		return "", err
	}
	path := helper.Name()
	if _, err = io.Copy(helper, source); err != nil {
		helper.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err = helper.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err = os.Chmod(path, 0700); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func RunHelper(encoded string) error {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	var payload helperPayload
	if err = json.Unmarshal(data, &payload); err != nil {
		return err
	}
	for i := 0; i < 1200; i++ {
		exited, checkErr := processExited(payload.PID)
		if checkErr != nil {
			return fmt.Errorf("检查主程序退出状态失败：%w", checkErr)
		}
		if exited {
			break
		}
		time.Sleep(250 * time.Millisecond)
		if i == 1199 {
			return fmt.Errorf("等待主程序退出超时")
		}
	}
	// 给 Windows 文件系统和杀毒软件一点时间释放 EXE 映像锁。
	time.Sleep(500 * time.Millisecond)
	if err = InstallArchive(payload.Archive, payload.Executable); err != nil {
		return fmt.Errorf("替换可执行文件失败：%w", err)
	}
	_ = os.Remove(payload.Archive)
	var startErr error
	for attempt := 0; attempt < 20; attempt++ {
		startErr = startUpdatedProcess(payload.Executable, payload.Args)
		if startErr == nil {
			scheduleHelperCleanup(payload.Helper)
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("启动新客户端失败：%w", startErr)
}

func HelperArguments(args []string) (string, bool) {
	if len(args) == 2 && args[0] == "__apply-update" {
		return args[1], true
	}
	return "", false
}
