//go:build !unix

package accountusage

import "os/exec"

func configureAgyCmd(*exec.Cmd) {}
