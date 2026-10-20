// history-orphans prints aggregate orphan counts from an existing SQLite file.
// It opens mode=ro, does not migrate, and has no repair operation.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
)

func main() {
	path := flag.String("db", "", "existing history.db path (read only)")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := history.InspectOrphans(ctx, *path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
