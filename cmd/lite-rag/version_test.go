package main

import (
	"bytes"
	"testing"
)

// The shared homebrew formula template — used by every tool in the org — runs
// `<binary> --version` in its test block, which is how the missing flag was
// found. Both spellings must work, and must print the same string.
//
// Deliberately reads the production wiring instead of assigning rootCmd.Version
// itself: that assignment is the fix, so a test that repeats it would pass
// against the broken binary.
func TestVersionFlagAndSubcommandAgree(t *testing.T) {
	if rootCmd.Version == "" {
		t.Fatal("rootCmd.Version is empty: cobra never registers --version, so " +
			"`--version` fails with \"unknown flag\" and `brew test` fails with it")
	}
	if rootCmd.Version != version {
		t.Errorf("rootCmd.Version = %q, want the linker-injected version %q", rootCmd.Version, version)
	}

	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})

	exec := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		if err := run(args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out.String()
	}

	flag := exec("--version")
	sub := exec("version")

	if want := "lite-rag " + version + "\n"; flag != want {
		t.Errorf("--version printed %q, want %q", flag, want)
	}
	if flag != sub {
		t.Errorf("--version printed %q but `version` printed %q; they must agree", flag, sub)
	}
}
