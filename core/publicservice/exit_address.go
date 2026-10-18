package publicservice

import (
	"context"
	"net/http"
	"time"
)

// ipify proves an address only. In particular a missing country/risk result is
// not an IPv4 failure, and the physical node transport family is not the exit.
func checkExitAddress(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result := checkIPify(ctx, client, rule, started)
	if result.Details == nil {
		result.Details = map[string]string{}
	}
	result.Details["provider"] = "ipify"
	result.Details["profile_fields"] = "ip_only"
	result.Details["query_at"] = started.UTC().Format(time.RFC3339Nano)
	result.Details["ip_family"] = "IPv4"
	if rule.ServiceID == "exit_ipv6" {
		result.Details["ip_family"] = "IPv6"
	}
	if result.Outcome == "profiled" {
		result.Details["exit_observed_at"] = result.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	return result
}
