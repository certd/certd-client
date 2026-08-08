//go:build !windows

package elevation

func newElevation() Elevation {
	return Elevation{
		isElevated: func() (bool, error) { return true, nil },
	}
}
