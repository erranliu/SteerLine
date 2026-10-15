// Package applicationclient reconnects only through verified home discovery.
package applicationclient

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	Home     string
	identity contract.Identity
}

func Open(ctx context.Context, home string) (*Client, error) {
	c := &Client{Home: home}
	id, err := c.RefreshIdentity(ctx)
	if err != nil {
		return nil, err
	}
	c.identity = id
	return c, nil
}
func (c *Client) Identity() contract.Identity { return c.identity }
func (c *Client) Close() error                { return nil }
func (c *Client) connect(ctx context.Context) (*localservice.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	client, err := localservice.Open(ctx, c.Home, filepath.Join(c.Home, "application-api"), contract.Protocol, "/v1/application/identity")
	if err != nil {
		return nil, err
	}
	if c.identity.HomeID != "" && client.Identity.HomeID != c.identity.HomeID {
		client.Close()
		return nil, errors.New("application home identity changed")
	}
	return client, nil
}
func (c *Client) RefreshIdentity(ctx context.Context) (contract.Identity, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	client, err := c.connect(ctx)
	if err != nil {
		return contract.Identity{}, err
	}
	defer client.Close()
	return client.Identity, nil
}
func valid(route string) bool {
	u, err := url.Parse(route)
	return err == nil && !u.IsAbs() && u.Host == "" && u.RawPath == "" && (u.Path == "/projects" || strings.HasPrefix(u.Path, "/projects/")) && !strings.ContainsAny(u.Path, "\\%") && !strings.Contains(u.Path, "..") && !strings.Contains(u.Path, "//")
}

func (c *Client) Get(ctx context.Context, path string, target any) error {
	if !valid(path) {
		return errors.New("invalid application path")
	}
	client, err := c.connect(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Request(ctx, "GET", "/v1/application"+path, nil, target)
}
func (c *Client) Post(ctx context.Context, path string, body, target any) error {
	if !valid(path) {
		return errors.New("invalid application path")
	}
	client, err := c.connect(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Request(ctx, "POST", "/v1/application"+path, body, target)
}
