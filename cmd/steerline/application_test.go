package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	app "github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	testprotocol "github.com/erranliu/SteerLine/internal/testkit/protocol"
)

func TestMain(m *testing.M) {
	if os.Getenv("STEERLINE_CLI_WORKER_FIXTURE") == "1" {
		switch strings.Join(os.Args[1:], " ") {
		case "--version":
			fmt.Println("codex-cli 0.159.2")
			return
		case "app-server --help":
			fmt.Println("app-server --listen")
			return
		case "app-server --stdio":
			testprotocol.MockAppServerStdio()
			return
		}
	}
	os.Exit(m.Run())
}

func startCLIHost(t *testing.T, config string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.ServeApplication(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(30 * time.Second):
			t.Error("project host did not close")
		}
	})
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		client, err := applicationclient.Open(ctx, app.HomeDir())
		if err == nil {
			var catalog contract.Catalog
			err = client.Get(ctx, "/projects", &catalog)
			client.Close()
			if err == nil {
				for _, project := range catalog.Projects {
					if project.ConfigID != filepath.Base(config) {
						continue
					}
					if project.RuntimeState == "failed" {
						t.Fatal(project.Diagnostic)
					}
					if project.Online {
						return
					}
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("required CLI project not ready")
}
