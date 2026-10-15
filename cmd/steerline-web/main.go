package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	server "github.com/erranliu/SteerLine/web/server"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

func home() string {
	if p := os.Getenv("STEERLINE_HOME"); p != "" {
		p, _ = filepath.Abs(p)
		return p
	}
	if runtime.GOOS == "windows" {
		return `C:\SteerlineRuntime`
	}
	p, _ := os.UserHomeDir()
	return filepath.Join(p, ".steerline")
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "steerline-web:", err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: steerline-web <serve|stop|open|install>")
	}
	flags := flag.NewFlagSet(os.Args[1], flag.ContinueOnError)
	project := flags.String("project", "", "project ID for opening")
	printURL := flags.Bool("print-url", false, "print one-use opening URL")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch os.Args[1] {
	case "serve":
		connectCtx, done := context.WithTimeout(ctx, 3*time.Second)
		client, err := applicationclient.Open(connectCtx, home())
		done()
		if err != nil {
			return fmt.Errorf("start steerline serve-app first: %w", err)
		}
		address, err := server.EnsureAddress(home())
		if err != nil {
			return err
		}
		web, err := server.Prepare(server.Options{Home: home(), Address: address, LauncherDir: server.LauncherDirectory(home()), Client: client, DevOrigin: os.Getenv("STEERLINE_WEB_DEV_ORIGIN")})
		if err != nil {
			return err
		}
		web.Start()
		fmt.Println("Web listening at http://" + address)
		select {
		case <-ctx.Done():
		case <-web.Done():
		}
		closeCtx, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		return web.Close(closeCtx)
	case "stop":
		ctx, done := context.WithTimeout(ctx, 45*time.Second)
		defer done()
		return server.Stop(ctx, home())
	case "open":
		url, err := server.OpeningURL(ctx, home(), *project)
		if err != nil {
			return fmt.Errorf("start steerline-web serve first: %w", err)
		}
		if *printURL {
			fmt.Println(url)
			return nil
		}
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
		case "darwin":
			cmd = exec.Command("open", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}
		if err = cmd.Start(); err != nil {
			return err
		}
		go cmd.Wait()
		return nil
	case "install":
		source, err := os.Executable()
		if err != nil {
			return err
		}
		name := "steerline-web"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		target, err := localservice.Install(home(), source, name)
		if err != nil {
			return err
		}
		fmt.Println("installed", target)
		return nil
	default:
		return errors.New("unknown Web command")
	}
}
