package cmd

import (
	"os"
	"testing"
)

// "--link work" must read work as the profile, not as an item to install.
func TestInstallLinkFlagTakesValue(t *testing.T) {
	t.Cleanup(func() { sourceInstallLink = "" })
	if err := installCmd.ParseFlags([]string{"owner/repo", "skills/x", "--link", "work"}); err != nil {
		t.Fatal(err)
	}
	if sourceInstallLink != "work" {
		t.Errorf("--link = %q, want work", sourceInstallLink)
	}
	if args := installCmd.Flags().Args(); len(args) != 2 {
		t.Errorf("positional args = %v, want [owner/repo skills/x]", args)
	}
}

func TestResolveLinkProfile(t *testing.T) {
	paths, _ := setupDoctorTest(t)
	t.Cleanup(func() { sourceInstallLink = "" })

	cases := []struct {
		link    string
		active  bool
		want    string
		wantErr bool
	}{
		{link: "", want: ""},
		{link: "p", want: "p"},
		{link: "nosuch", wantErr: true},
		{link: ".", wantErr: true}, // no active profile yet
		{link: ".", active: true, want: "p"},
	}
	for _, c := range cases {
		if c.active {
			os.Symlink("profiles/p", paths.ClaudeDir)
		}
		sourceInstallLink = c.link
		got, err := resolveLinkProfile(paths)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("--link %q (active=%v): got %q, err %v", c.link, c.active, got, err)
		}
	}
}

// --link means nothing when syncing ccp.toml; say so instead of ignoring it.
func TestInstallLinkRejectedInSyncMode(t *testing.T) {
	sourceInstallLink = "p"
	t.Cleanup(func() { sourceInstallLink = "" })
	if err := runSourceInstall(installCmd, nil); err == nil {
		t.Error("expected an error for --link without a package")
	}
}
