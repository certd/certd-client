package elevation

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsElevatorRelaunchesWithRunAs(t *testing.T) {
	var verb, executable, parameters string
	elevator := windowsElevator{
		shellExecute: func(_ windows.Handle, actualVerb, actualExecutable, actualParameters, _ *uint16, showCommand int32) error {
			verb = windows.UTF16PtrToString(actualVerb)
			executable = windows.UTF16PtrToString(actualExecutable)
			parameters = windows.UTF16PtrToString(actualParameters)
			if showCommand != windows.SW_SHOWNORMAL {
				t.Fatalf("unexpected show command: %d", showCommand)
			}
			return nil
		},
	}
	arguments := []string{"--config", "C:\\Certd Data\\client.yaml"}

	if err := elevator.relaunch("C:\\Program Files\\Certd\\certd-client.exe", arguments); err != nil {
		t.Fatal(err)
	}
	if verb != "runas" || executable != "C:\\Program Files\\Certd\\certd-client.exe" || parameters != windows.ComposeCommandLine(arguments) {
		t.Fatalf("unexpected elevated restart: verb=%q executable=%q parameters=%q", verb, executable, parameters)
	}
}

func TestCurrentProcessIsElevated(t *testing.T) {
	if _, err := currentProcessIsElevated(); err != nil {
		t.Fatalf("check current process elevation: %v", err)
	}
}

func TestNewInitializesWindowsElevation(t *testing.T) {
	controller := New()
	if controller.isElevated == nil || controller.executable == nil || controller.arguments == nil || controller.relaunch == nil {
		t.Fatalf("incomplete Windows elevation controller: %#v", controller)
	}
}
