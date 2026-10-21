//go:build windows

package discovery

import (
	"context"
	"github.com/Microsoft/go-winio"
	"net"
)

func dialLocalOrca(ctx context.Context, endpoint string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, endpoint)
}
