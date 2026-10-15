package application

import "context"

type probeTimingContextKey struct{}

// Values are copied at admission; runtime goroutines never share a mutable map.
func probeTiming(ctx context.Context) map[string]int64 {
	out := map[string]int64{}
	if ctx == nil {
		return out
	}
	if v, ok := ctx.Value(probeTimingContextKey{}).(map[string]int64); ok {
		for k, n := range v {
			out[k] = n
		}
	}
	return out
}
func withProbeTiming(ctx context.Context, key string, n int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	v := probeTiming(ctx)
	v[key] = n
	return context.WithValue(ctx, probeTimingContextKey{}, v)
}
