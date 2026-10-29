//go:build !windows

package control

import "time"

func processCPUTime() (time.Duration, bool) { return 0, false }
