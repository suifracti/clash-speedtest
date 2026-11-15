//go:build !darwin && !windows

package profiles

import (
	"context"
	"net/url"
)

func normalSystemProxy(context.Context, string) (*url.URL, error) { return nil, nil }
