package elevation

import (
	"os"

	"golang.org/x/sys/windows"
)

type windowsElevator struct {
	shellExecute func(windows.Handle, *uint16, *uint16, *uint16, *uint16, int32) error
}

func newElevation() Elevation {
	elevator := windowsElevator{shellExecute: windows.ShellExecute}
	return Elevation{
		isElevated: currentProcessIsElevated,
		executable: os.Executable,
		arguments: func() []string {
			return append([]string(nil), os.Args[1:]...)
		},
		relaunch: elevator.relaunch,
	}
}

func currentProcessIsElevated() (bool, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false, err
	}
	defer token.Close()
	return token.IsElevated(), nil
}

func (elevator windowsElevator) relaunch(executable string, arguments []string) error {
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	parameters, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(arguments))
	if err != nil {
		return err
	}
	return elevator.shellExecute(0, verb, file, parameters, nil, windows.SW_SHOWNORMAL)
}
