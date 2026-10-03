package main

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestElevationQuotesNativeCommandAndOriginalUser(t *testing.T) {
	t.Setenv("USERNAME", "original-user")
	exe := `C:\Program Files\Codex\codex.exe`
	home := `C:\owned user's $home\codex`
	cmd, err := provisioningCommand(exe, home, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	script := cmd.Args[len(cmd.Args)-1]
	if !strings.Contains(script, "-Verb RunAs -WindowStyle Hidden") || !strings.Contains(script, "WaitForExit(60000)") || !strings.Contains(script, "'C:\\Program Files\\Codex\\codex.exe'") {
		t.Fatalf("unexpected elevation command: %s", script)
	}
	args := windows.ComposeCommandLine([]string{"sandbox", "setup", "--elevated", "--user", "original-user", "--codex-home", home})
	if !strings.Contains(script, "'"+strings.ReplaceAll(args, "'", "''")+"'") {
		t.Fatal("Windows argv or PowerShell literal escaping changed")
	}
}
