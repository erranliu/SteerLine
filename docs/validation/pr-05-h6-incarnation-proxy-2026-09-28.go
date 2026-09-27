package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const realOrca = `C:\Users\lmas1\AppData\Local\Programs\orca\resources\bin\orca.exe`
const replacementIncarnation = "h-a6-injected-replacement-20260928"

func main() {
	args := os.Args[1:]
	root := filepath.Dir(os.Args[0])
	command := exec.Command(realOrca, args...)
	command.Stdin = os.Stdin
	if len(args) < 2 || args[0] != "terminal" || args[1] != "show" {
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		if err := command.Run(); err != nil {
			exit(err)
		}
		return
	}
	output, err := command.CombinedOutput()
	if err != nil {
		os.Stdout.Write(output)
		exit(err)
	}
	arm := filepath.Join(root, "replace-incarnation.arm")
	active := filepath.Join(root, "replace-incarnation.active")
	targetBytes, readErr := os.ReadFile(active)
	first := false
	if readErr != nil {
		targetBytes, readErr = os.ReadFile(arm)
		first = readErr == nil
	}
	if readErr != nil {
		os.Stdout.Write(output)
		return
	}
	target := strings.TrimSpace(string(targetBytes))
	var receipt struct {
		OK     bool `json:"ok"`
		Result struct {
			Terminal struct {
				Handle        string `json:"handle"`
				IncarnationID string `json:"incarnationId"`
			} `json:"terminal"`
		} `json:"result"`
	}
	if json.Unmarshal(output, &receipt) != nil || !receipt.OK || target == "" || receipt.Result.Terminal.Handle != target {
		os.Stdout.Write(output)
		return
	}
	var document map[string]any
	if err := json.Unmarshal(output, &document); err != nil {
		panic(err)
	}
	result := document["result"].(map[string]any)
	terminal := result["terminal"].(map[string]any)
	terminal["incarnationId"] = replacementIncarnation
	mutated, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}
	if first {
		if err := os.WriteFile(filepath.Join(root, "first-original-show.json"), output, 0600); err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(root, "first-mutated-show.json"), mutated, 0600); err != nil {
			panic(err)
		}
		if err := os.Rename(arm, active); err != nil {
			panic(err)
		}
	}
	os.Stdout.Write(mutated)
}

func exit(err error) {
	if status, ok := err.(*exec.ExitError); ok {
		os.Exit(status.ExitCode())
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
