//go:build windows

package control

import (
	"golang.org/x/sys/windows"
	"time"
)

func processCPUTime() (time.Duration, bool) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user); err != nil {
		return 0, false
	}
	ticks := (uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime)) + (uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime))
	return time.Duration(ticks) * 100 * time.Nanosecond, true
}
