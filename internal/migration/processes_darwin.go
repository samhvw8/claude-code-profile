//go:build darwin

package migration

import (
	"errors"

	"golang.org/x/sys/unix"
)

func newSysctlLister() ProcessLister {
	return sysctlLister{procs: kernProcAll, procargs: kernProcargs2}
}

func kernProcAll() ([]procEntry, error) {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	out := make([]procEntry, 0, len(kps))
	for _, kp := range kps {
		out = append(out, procEntry{pid: int(kp.Proc.P_pid), comm: unix.ByteSliceToString(kp.Proc.P_comm[:])})
	}
	return out, nil
}

func kernProcargs2(pid int) ([]byte, error) {
	buf, err := unix.SysctlRaw("kern.procargs2", pid)
	if errors.Is(err, unix.ESRCH) {
		return nil, errProcessGone
	}
	return buf, err
}
