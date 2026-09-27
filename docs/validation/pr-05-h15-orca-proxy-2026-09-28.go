package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const realOrca = `C:\Users\lmas1\AppData\Local\Programs\orca\resources\bin\orca.exe`

func main() {
	args := os.Args[1:]
	root := filepath.Dir(os.Args[0])
	showArm := filepath.Join(root, "drop-next-terminal-show.arm")
	if len(args) >= 2 && args[0] == "terminal" && args[1] == "show" {
		if _, err := os.Stat(showArm); err == nil {
			if err := os.Rename(showArm, filepath.Join(root, "drop-terminal-show-used.marker")); err != nil {
				panic(err)
			}
			fmt.Fprintln(os.Stderr, "injected transient terminal-show response loss")
			os.Exit(75)
		}
	}
	arm := filepath.Join(root, "drop-next-terminal-create.arm")
	inject := len(args) >= 2 && args[0] == "terminal" && args[1] == "create"
	if inject {
		if _, err := os.Stat(arm); err == nil {
			command := exec.Command(realOrca, args...)
			command.Stdin = os.Stdin
			output, runErr := command.CombinedOutput()
			var receipt struct{ OK bool `json:"ok"` }
			if runErr == nil && json.Unmarshal(output, &receipt) == nil && receipt.OK {
				if err := os.WriteFile(filepath.Join(root, "dropped-terminal-create-receipt.json"), output, 0600); err != nil {
					panic(err)
				}
				if err := os.Rename(arm, filepath.Join(root, "drop-terminal-create-used.marker")); err != nil {
					panic(err)
				}
				fmt.Fprintln(os.Stderr, "injected terminal-create response loss after Orca accepted it")
				os.Exit(75)
			}
			os.Stdout.Write(output)
			if runErr != nil {
				if status, ok := runErr.(*exec.ExitError); ok {
					os.Exit(status.ExitCode())
				}
				os.Exit(1)
			}
			return
		}
	}
	command := exec.Command(realOrca, args...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		if status, ok := err.(*exec.ExitError); ok {
			os.Exit(status.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
