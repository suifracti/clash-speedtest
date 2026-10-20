package history

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLitePragmasAndCascadeOnEveryConnection(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "quota #& space")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	d, err := OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d.db.SetMaxOpenConns(4)
	// Holding every connection forces sql.DB to open new physical connections.
	var conns []*sql.Conn
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	for i := 0; i < 4; i++ {
		c, err := d.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	for i, c := range conns {
		for pragma, want := range map[string]int{"foreign_keys": 1, "busy_timeout": 5000, "synchronous": 1} {
			var got int
			if err := c.QueryRowContext(ctx, "PRAGMA "+pragma).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("connection %d: %s=%d, want %d", i, pragma, got, want)
			}
		}
		id, now := fmt.Sprintf("fixture-%d", i), time.Now().UTC()
		_, err := c.ExecContext(ctx, `INSERT INTO workbench_latency_tests (attempt_id,profile_id,node_key,node_identity_key,config_revision_key,display_name,node_type,test_project,requested_at,started_at,finished_at,status) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, id, "profile", "node", "identity", "revision", "fixture", "direct", "latency", now, now, now, "completed")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.ExecContext(ctx, `INSERT INTO workbench_latency_samples (attempt_id,seq,timestamp,success) VALUES (?,0,?,1)`, id, now); err != nil {
			t.Fatal(err)
		}
		if _, err := c.ExecContext(ctx, `DELETE FROM workbench_latency_tests WHERE attempt_id=?`, id); err != nil {
			t.Fatal(err)
		}
		var remaining int
		if err := c.QueryRowContext(ctx, `SELECT COUNT(*) FROM workbench_latency_samples WHERE attempt_id=?`, id).Scan(&remaining); err != nil {
			t.Fatal(err)
		}
		if remaining != 0 {
			t.Errorf("connection %d left %d orphan samples after deleting parent", i, remaining)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "history.db")); err != nil {
		t.Fatal(err)
	}
}
