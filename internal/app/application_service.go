package app

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/applicationhttp"
	"net/http"
	"path/filepath"
	"time"
)

func ServeApplication(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	endpoint, err := localservice.Prepare(HomeDir(), filepath.Join(HomeDir(), "application-api"), contract.Protocol, "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer endpoint.Close()
	directory := NewApplicationDirectory(HomeDir())
	directory.Start()
	server := &http.Server{Handler: &applicationhttp.Server{Endpoint: endpoint, Directory: directory, Shutdown: func() { directory.BeginStop(); cancel() }}, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 125 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	done := make(chan error, 1)
	go func() { done <- server.Serve(endpoint.Listener) }()
	select {
	case <-ctx.Done():
	case err = <-done:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}
	directory.BeginStop()
	projectErr := directory.Close()
	result := contract.Shutdown{Version: 1, Identity: endpoint.Identity, Projects: directory.ShutdownResults(), Clean: projectErr == nil}
	if projectErr != nil {
		result.Diagnostic = boundedDiagnostic(projectErr.Error(), 4096)
	}
	for i := range result.Projects {
		result.Projects[i].Diagnostic = boundedDiagnostic(result.Projects[i].Diagnostic, 512)
	}
	for _, project := range result.Projects {
		if !project.LocalClosed || !project.LocksReleased || !project.ResourcesSettled {
			result.Clean = false
		}
	}
	if err := localservice.Write(endpoint.Dir, "shutdown.json", result); err != nil {
		return err
	}
	for _, project := range result.Projects {
		if !project.LocalClosed { // retain ownership after an unverified local close
			<-make(chan struct{})
		}
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer closeCancel()
	shutdown := server.Shutdown(closeCtx)
	return errors.Join(err, projectErr, shutdown, server.Close())
}
func StopApplication(ctx context.Context) error {
	return stopBackend(ctx, HomeDir())
}

func boundedDiagnostic(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return string([]rune(value[:limit])) + "…"
}
