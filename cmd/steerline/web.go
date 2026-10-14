package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/erranliu/SteerLine/internal/app"
	webserver "github.com/erranliu/SteerLine/web/server"
)

func runWeb(args []string) error {
	flags := flag.NewFlagSet("web", flag.ContinueOnError)
	configPath := flags.String("config", "steerline.local.json", "project configuration")
	printURL := flags.Bool("print-url", false, "print a one-use opening URL for controlled browser automation")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected Web arguments")
	}
	config, err := app.LoadQueryConfig(*configPath)
	if err != nil {
		return err
	}
	if err := config.ResolveIdentity(context.Background()); err != nil {
		return err
	}
	if config.WebAddress == "" {
		return errors.New("Web is disabled; configure web_address and explicitly restart the existing service")
	}
	target, err := webserver.OpeningURL(context.Background(), config.RunDir, config.Identity(), config.WebAddress, webserver.LauncherDirectory(app.HomeDir(), config.Identity()))
	if err != nil {
		return err
	}
	if *printURL {
		fmt.Println(target)
		return nil
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", target)
	case "darwin":
		command = exec.Command("open", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	if err := command.Start(); err != nil {
		return err
	}
	go command.Wait()
	fmt.Println("opened Web for", config.Identity())
	return nil
}
