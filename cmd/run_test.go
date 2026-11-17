package cmd

import (
	"slices"
	"testing"
)

func TestParseRunArgs(t *testing.T) {
	tests := []struct {
		args    []string
		want    runArgs
		wantErr bool
	}{
		{args: []string{"dev", "--", "claude", "-p", "hi"}, want: runArgs{profile: "dev", command: []string{"claude", "-p", "hi"}}},
		{args: []string{"default", "--account", "work1", "--", "claude"}, want: runArgs{profile: "default", account: "work1", command: []string{"claude"}}},
		{args: []string{"--account=work1", "--relink", "default", "--", "claude", "--account", "x"}, want: runArgs{profile: "default", account: "work1", relink: true, command: []string{"claude", "--account", "x"}}},
		{args: []string{"dev", "--relink", "--", "claude"}, wantErr: true},
		{args: []string{"dev", "--account", "--", "claude"}, wantErr: true},
		{args: []string{"dev", "--bogus", "--", "claude"}, wantErr: true},
		{args: []string{"dev", "extra", "--", "claude"}, wantErr: true},
		{args: []string{"dev", "--"}, wantErr: true},
		{args: []string{"dev", "claude"}, wantErr: true},
		{args: []string{"--", "claude"}, wantErr: true},
	}
	for _, tt := range tests {
		got, err := parseRunArgs(tt.args)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseRunArgs(%q) err = %v, wantErr %v", tt.args, err, tt.wantErr)
			continue
		}
		if tt.wantErr {
			continue
		}
		if got.profile != tt.want.profile || got.account != tt.want.account || got.relink != tt.want.relink || !slices.Equal(got.command, tt.want.command) {
			t.Errorf("parseRunArgs(%q) = %+v, want %+v", tt.args, got, tt.want)
		}
	}
}
