//go:build !windows && !darwin

package speedtester

func physicalGatewayMetrics() map[int]uint32 { return nil }
