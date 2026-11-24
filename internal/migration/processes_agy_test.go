package migration

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAppDataDirOf(t *testing.T) {
	tests := []struct {
		name      string
		argv      []string
		wantDir   string
		wantFound bool
	}{
		{"double_dash_eq", []string{"agy", "--app_data_dir=antigravity-cli-ccp-a1"}, "antigravity-cli-ccp-a1", true},
		{"single_dash_eq", []string{"agy", "-app_data_dir=antigravity-cli-ccp-a1"}, "antigravity-cli-ccp-a1", true},
		{"double_dash_sep", []string{"agy", "--app_data_dir", "antigravity-cli-ccp-a1"}, "antigravity-cli-ccp-a1", true},
		{"single_dash_sep", []string{"agy", "-app_data_dir", "antigravity-cli-ccp-a1"}, "antigravity-cli-ccp-a1", true},
		{"after_terminator_ignored", []string{"agy", "--", "--app_data_dir=antigravity-cli-ccp-a1"}, "", false},
		{"no_flag", []string{"agy", "-p", "/usage"}, "", false},
		{"env_wrapper", []string{"/usr/bin/env", "agy", "--app_data_dir=antigravity-cli-ccp-a2"}, "antigravity-cli-ccp-a2", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := appDataDirOf(tc.argv)
			if ok != tc.wantFound || got != tc.wantDir {
				t.Errorf("appDataDirOf(%v) = (%q, %v), want (%q, %v)", tc.argv, got, ok, tc.wantDir, tc.wantFound)
			}
		})
	}
}

func makeProcargs2(execPath string, argv []string) []byte {
	var buf []byte
	var hdr [4]byte
	binary.NativeEndian.PutUint32(hdr[:], uint32(len(argv)))
	buf = append(buf, hdr[:]...)
	buf = append(buf, execPath...)
	buf = append(buf, 0, 0)
	for _, a := range argv {
		buf = append(buf, a...)
		buf = append(buf, 0)
	}
	buf = append(buf, 0)
	return buf
}

func TestSysctlLister_AgyProcesses(t *testing.T) {
	l := sysctlLister{
		procs: func() ([]procEntry, error) {
			return []procEntry{
				{pid: 10, comm: "agy"},
				{pid: 20, comm: "agy"},
				{pid: 30, comm: "zsh"},
				{pid: 40, comm: "antigravity-cli"},
			}, nil
		},
		procargs: func(pid int) ([]byte, error) {
			switch pid {
			case 10:
				return makeProcargs2("/usr/local/bin/agy", []string{"agy", "--app_data_dir=antigravity-cli-ccp-a1"}), nil
			case 20:
				return nil, errProcessGone
			case 30:
				return makeProcargs2("/bin/zsh", []string{"zsh"}), nil
			case 40:
				return nil, errors.New("EPERM")
			}
			return nil, errProcessGone
		},
	}
	got, err := l.AgyProcesses()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v, want 2 entries", got)
	}
	if got[0].PID != 10 || got[0].AppDataDir != "antigravity-cli-ccp-a1" || !got[0].ArgsKnown {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].PID != 40 || got[1].ArgsKnown {
		t.Errorf("got[1] = %+v, want PID 40 with ArgsKnown=false", got[1])
	}
}

func TestProcLister_AgyProcesses(t *testing.T) {
	root := t.TempDir()
	p1 := filepath.Join(root, "101")
	if err := os.MkdirAll(p1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p1, "cmdline"), []byte("agy\x00--app_data_dir\x00antigravity-cli-ccp-a1\x00"), 0644); err != nil {
		t.Fatal(err)
	}
	p2 := filepath.Join(root, "102")
	if err := os.MkdirAll(p2, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p2, "cmdline"), []byte("bash\x00-l\x00"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := procLister{root: root}.AgyProcesses()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PID != 101 || got[0].AppDataDir != "antigravity-cli-ccp-a1" || !got[0].ArgsKnown {
		t.Fatalf("got %+v, want single PID 101", got)
	}
}
