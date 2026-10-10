package main

import (
	"errors"
	"flag"
	"github.com/faceair/clash-speedtest/adapter/desktop"
	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/appdata"
	"testing"
)

func TestNoAutoCredentialsFlagDefaultsToFalse(t *testing.T) {
	if value := flag.Lookup("no-auto-credentials"); value == nil || value.DefValue != "false" {
		t.Fatalf("incorrect default: %+v", value)
	}
}

func TestListenFlagDefaultsToLoopback(t *testing.T) {
	if value := flag.Lookup("listen"); value == nil || value.DefValue != "127.0.0.1" {
		t.Fatalf("incorrect listen default: %+v", value)
	}
}

func TestNoAutoCredentialsNativeFailureKeepsIsolationInWebFallback(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "isolated"}[disabled], func(t *testing.T) {
			paths, err := appdata.Resolve(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			native, web := 0, 0
			opts := application.Options{NoAutoCredentials: disabled}
			access := webAccessOptions{
				listenAddress: "192.0.2.10",
				publicIPv6:    "2001:4860:4860::8888",
				webPassword:   "xxxxxxxxxxxx",
			}
			runDesktopWithRunners("fixture-ua", 4321, "none", paths, opts, access, desktopRunners{
				startNative: func(cfg desktop.RunConfig) error {
					native++
					if cfg.AppOptions.NoAutoCredentials != disabled || cfg.AppPaths != paths {
						t.Fatal("native config lost startup options")
					}
					return errors.New("injected native failure")
				},
				startWeb: func(port int, ua, browser string, p appdata.AppPaths, o application.Options, gotAccess webAccessOptions) {
					web++
					if o.NoAutoCredentials != disabled || p != paths || port != 4321 || browser != "none" || ua != "fixture-ua" {
						t.Fatal("fallback lost startup context")
					}
					if gotAccess.listenAddress != "127.0.0.1" || gotAccess.publicIPv6 != "" || gotAccess.webPassword != access.webPassword {
						t.Fatal("fallback access was not forced local while preserving configured authentication")
					}
				},
			})
			if native != 1 || web != 1 {
				t.Fatalf("native=%d web=%d", native, web)
			}
		})
	}
}
func TestNoAutoCredentialsNativeSuccessDoesNotStartFallback(t *testing.T) {
	paths, _ := appdata.Resolve(t.TempDir())
	web := 0
	runDesktopWithRunners("", 0, "none", paths, application.Options{NoAutoCredentials: true}, webAccessOptions{}, desktopRunners{
		startNative: func(cfg desktop.RunConfig) error {
			if !cfg.AppOptions.NoAutoCredentials {
				t.Fatal("isolation lost")
			}
			return nil
		},
		startWeb: func(int, string, string, appdata.AppPaths, application.Options, webAccessOptions) { web++ },
	})
	if web != 0 {
		t.Fatalf("unexpected fallback calls=%d", web)
	}
}
