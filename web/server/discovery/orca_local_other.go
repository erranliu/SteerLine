//go:build !windows

package discovery

import (
	"context"
	"errors"
	"net"
)

func dialLocalOrca(context.Context, string) (net.Conn, error) {
	return nil, errors.New("local Orca discovery requires Windows named pipes")
}
