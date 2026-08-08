package elevation

import (
	"errors"
	"reflect"
	"testing"
)

func TestRequestDoesNotRestartWhenAlreadyElevated(t *testing.T) {
	restarted := false
	controller := Elevation{
		isElevated: func() (bool, error) { return true, nil },
		relaunch: func(string, []string) error {
			restarted = true
			return nil
		},
	}

	relaunched, err := controller.Request()
	if err != nil {
		t.Fatal(err)
	}
	if relaunched || restarted {
		t.Fatal("elevated process must not restart")
	}
}

func TestRequestRestartsWithCurrentExecutableAndArguments(t *testing.T) {
	var executable string
	var arguments []string
	controller := Elevation{
		isElevated: func() (bool, error) { return false, nil },
		executable: func() (string, error) { return "C:\\Program Files\\Certd\\certd-client.exe", nil },
		arguments:  func() []string { return []string{"--config", "C:\\Certd Data\\client.yaml"} },
		relaunch: func(path string, args []string) error {
			executable = path
			arguments = args
			return nil
		},
	}

	relaunched, err := controller.Request()
	if err != nil {
		t.Fatal(err)
	}
	if !relaunched || executable != "C:\\Program Files\\Certd\\certd-client.exe" || !reflect.DeepEqual(arguments, []string{"--config", "C:\\Certd Data\\client.yaml"}) {
		t.Fatalf("unexpected UAC restart: relaunched=%v executable=%q arguments=%#v", relaunched, executable, arguments)
	}
}

func TestRequestReturnsRelaunchError(t *testing.T) {
	controller := Elevation{
		isElevated: func() (bool, error) { return false, nil },
		executable: func() (string, error) { return "certd-client.exe", nil },
		arguments:  func() []string { return nil },
		relaunch: func(string, []string) error {
			return errors.New("UAC was cancelled")
		},
	}

	_, err := controller.Request()
	if err == nil || err.Error() != "restart with administrator permission: UAC was cancelled" {
		t.Fatalf("unexpected relaunch error: %v", err)
	}
}

func TestRequestReturnsPermissionCheckError(t *testing.T) {
	controller := Elevation{
		isElevated: func() (bool, error) { return false, errors.New("access denied") },
	}

	_, err := controller.Request()
	if err == nil || err.Error() != "check administrator permission: access denied" {
		t.Fatalf("unexpected permission check error: %v", err)
	}
}

func TestRequestReturnsExecutableError(t *testing.T) {
	controller := Elevation{
		isElevated: func() (bool, error) { return false, nil },
		executable: func() (string, error) { return "", errors.New("executable unavailable") },
	}

	_, err := controller.Request()
	if err == nil || err.Error() != "get current executable: executable unavailable" {
		t.Fatalf("unexpected executable error: %v", err)
	}
}
