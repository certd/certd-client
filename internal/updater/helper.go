package updater

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/certd/certd-client/internal/logging"
)

type helperPayload struct {
	Archive, Executable, Helper string
	PID                         int
	Args                        []string
}

// CurrentExecutable returns the path of the running client executable.
func CurrentExecutable() (string, error) { return os.Executable() }

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
	logging.Info("更新 helper 已启动")
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	var payload helperPayload
	if err = json.Unmarshal(data, &payload); err != nil {
		return err
	}
	logging.Info("更新参数已解析，等待旧客户端退出")
	for i := 0; i < 1200; i++ {
		exited, checkErr := processExited(payload.PID)
		if checkErr != nil {
			return fmt.Errorf("检查主程序退出状态失败：%w", checkErr)
		}
		if exited {
			logging.Info("旧客户端已退出")
			break
		}
		time.Sleep(250 * time.Millisecond)
		if i == 1199 {
			logging.Info("等待旧客户端退出超时")
			return fmt.Errorf("等待主程序退出超时")
		}
	}
	// 给 Windows 文件系统和杀毒软件一点时间释放 EXE 映像锁。
	time.Sleep(500 * time.Millisecond)
	if err = InstallArchive(payload.Archive, payload.Executable); err != nil {
		logging.Error("替换客户端文件失败：%s", err)
		return fmt.Errorf("替换可执行文件失败：%w", err)
	}
	logging.Info("新版本文件替换完成")
	_ = os.Remove(payload.Archive)
	logging.Info("主程序更新完成，系统服务副本不受影响")
	if runtime.GOOS != "windows" {
		_ = os.Remove(payload.Helper)
		logging.Info("更新成功，请手动运行 ./certd-client 重新启动客户端")
		return nil
	}
	var startErr error
	for attempt := 0; attempt < 20; attempt++ {
		startErr = startUpdatedProcess(payload.Executable, payload.Args)
		if startErr == nil {
			logging.Info("新版本客户端已启动")
			scheduleHelperCleanup(payload.Helper)
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	logging.Error("启动新客户端失败：%s", startErr)
	return fmt.Errorf("启动新客户端失败：%w", startErr)
}

func HelperArguments(args []string) (string, bool) {
	if len(args) == 2 && args[0] == "__apply-update" {
		return args[1], true
	}
	return "", false
}
