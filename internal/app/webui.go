package app

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/erranliu/SteerLine/internal/product/httpclient"
	webserver "github.com/erranliu/SteerLine/web/server"
)

type serviceWeb struct{ *webserver.Server }

// Called only while serviceLocalAPI's lifecycle lock is held. Browser hosting
// consumes the published native client; it never opens a second business store.
func (s *serviceLocalAPI) startWeb() error {
	if s.webAddress == "" || s.web != nil {
		return nil
	}
	if err := webserver.ValidateAddress(s.webAddress); err != nil {
		return err
	}
	client, err := httpclient.OpenWithTimeout(context.Background(), s.runDir, s.projectID, 2*time.Minute)
	if err != nil {
		return err
	}
	server, err := webserver.Prepare(webserver.Options{Address: s.webAddress, RunDir: s.runDir, LauncherDir: webserver.LauncherDirectory(HomeDir(), s.projectID), Client: client, DevOrigin: os.Getenv("STEERLINE_WEB_DEV_ORIGIN")})
	if err != nil {
		return errors.Join(err, client.Close())
	}
	s.web = &serviceWeb{server}
	server.Start()
	return nil
}
