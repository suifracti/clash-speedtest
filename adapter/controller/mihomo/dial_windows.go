//go:build windows

package mihomo

import (
	"context"
	"net"
	"strings"

	"github.com/metacubex/wireguard-go/ipc/namedpipe"
)

func dialLocalSocket(ctx context.Context, socketPath string) (net.Conn, error) {
	if strings.HasPrefix(socketPath, `\\.\pipe\`) {
		conn, err := namedpipe.DialContext(ctx, socketPath)
		if err != nil {
			for _, oldPart := range []string{"-sidecar-release-", "-sidecar-"} {
				if strings.Contains(socketPath, oldPart) {
					alt := strings.Replace(socketPath, oldPart, "-production-", 1)
					if altConn, altErr := namedpipe.DialContext(ctx, alt); altErr == nil {
						return altConn, nil
					}
				}
			}
		}
		return conn, err
	}
	return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
}
