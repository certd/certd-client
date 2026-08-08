// Package elevation handles the Windows UAC restart required by local IIS operations.
package elevation

import "fmt"

type Elevation struct {
	isElevated func() (bool, error)
	executable func() (string, error)
	arguments  func() []string
	relaunch   func(string, []string) error
}

func New() Elevation { return newElevation() }

// Request restarts the current executable with administrator permission when needed.
// It reports whether a replacement process was started successfully.
func (controller Elevation) Request() (bool, error) {
	elevated, err := controller.isElevated()
	if err != nil {
		return false, fmt.Errorf("check administrator permission: %w", err)
	}
	if elevated {
		return false, nil
	}
	executable, err := controller.executable()
	if err != nil {
		return false, fmt.Errorf("get current executable: %w", err)
	}
	if err := controller.relaunch(executable, controller.arguments()); err != nil {
		return false, fmt.Errorf("restart with administrator permission: %w", err)
	}
	return true, nil
}
