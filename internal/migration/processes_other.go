//go:build !darwin

package migration

func newSysctlLister() ProcessLister { return nil }
