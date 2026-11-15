//go:build !windows

package mihomo

import (
	"context"
	"net"
)

func dialLocalSocket(ctx context.Context, socketPath string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
}
