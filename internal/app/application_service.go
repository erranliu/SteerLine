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
	defer directory.Close()
	server := &http.Server{Handler: &applicationhttp.Server{Endpoint: endpoint, Directory: directory, Shutdown: cancel}, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 125 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	done := make(chan error, 1)
	go func() { done <- server.Serve(endpoint.Listener) }()
	select {
	case <-ctx.Done():
	case err = <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer closeCancel()
	shutdown := server.Shutdown(closeCtx)
	if errors.Is(shutdown, context.DeadlineExceeded) {
		shutdown = nil
	}
	return errors.Join(shutdown, server.Close())
}
func StopApplication(ctx context.Context) error {
	return localservice.Stop(ctx, HomeDir(), filepath.Join(HomeDir(), "application-api"), contract.Protocol, "/v1/application/identity", "/v1/application/shutdown")
}
