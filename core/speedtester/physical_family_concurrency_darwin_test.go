//go:build darwin

package speedtester

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

func TestSettledPhysicalFamilyDoesNotSerializeIndependentTargetDials(t *testing.T) {
	path := familyPath("ipv4")
	path.udp = 1 // synthetic verified fixture; never opens a physical network socket
	entered := make(chan string, 3)
	adapter := &physicalFixtureProxy{connect: func(ctx context.Context, m *C.Metadata) (C.Conn, error) {
		if m.Host == "warmup.invalid" {
			c, s := familyMemoryStream()
			s.Close()
			return c, nil
		}
		entered <- m.Host
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	wrapped := wrapPhysicalFamilies(&CProxy{Proxy: adapter}, path, nil, nil, time.Second)
	defer wrapped.Close()
	conn, err := wrapped.DialContext(context.Background(), &C.Metadata{Host: "warmup.invalid", DstPort: 443})
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	done := make(chan error, 2)
	for _, host := range []string{"slow.invalid", "independent.invalid"} {
		go func(host string) {
			_, err := wrapped.DialContext(context.Background(), &C.Metadata{Host: host, DstPort: 443})
			done <- err
		}(host)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(200 * time.Millisecond):
			t.Fatal("one slow target serialized the other site's dial")
		}
	}
	wrapped.Close()
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("close did not cancel stream: %v", err)
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatal("pending stream survived proxy close")
		}
	}
	if adapter.closed.Load() != 1 {
		t.Fatal("adapter closed more than once")
	}
}

func TestWaitingPhysicalFamilySelectionObservesItsOwnCancellation(t *testing.T) {
	entered := make(chan struct{})
	adapter := &physicalFixtureProxy{connect: func(ctx context.Context, _ *C.Metadata) (C.Conn, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	wrapped := wrapPhysicalFamilies(&CProxy{Proxy: adapter}, familyPath("ipv4"), nil, nil, 2*time.Second)
	defer wrapped.Close()
	firstDone := make(chan error, 1)
	go func() {
		_, err := wrapped.DialContext(context.Background(), &C.Metadata{Host: "first.invalid"})
		firstDone <- err
	}()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	waiting := make(chan error, 1)
	go func() { _, err := wrapped.DialContext(ctx, &C.Metadata{Host: "waiting.invalid"}); waiting <- err }()
	cancel()
	select {
	case err := <-waiting:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("cancelled target waited for another site's selection")
	}
	wrapped.Close()
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("selection leaked")
	}
	if adapter.calls.Load() != 1 {
		t.Fatal("cancelled waiter made a new node connection")
	}
}

func TestPhysicalFamilyClosesLateSuccessfulStreamAfterCancellation(t *testing.T) {
	path := familyPath("ipv4")
	path.udp = 1
	entered := make(chan struct{})
	release := make(chan struct{})
	var server net.Conn
	adapter := &physicalFixtureProxy{connect: func(ctx context.Context, m *C.Metadata) (C.Conn, error) {
		c, s := familyMemoryStream()
		if m.Host == "warmup.invalid" {
			s.Close()
			return c, nil
		}
		server = s
		close(entered)
		<-release
		return c, nil
	}}
	wrapped := wrapPhysicalFamilies(&CProxy{Proxy: adapter}, path, nil, nil, time.Second)
	defer wrapped.Close()
	warm, err := wrapped.DialContext(context.Background(), &C.Metadata{Host: "warmup.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	warm.Close()
	done := make(chan error, 1)
	go func() {
		c, err := wrapped.DialContext(context.Background(), &C.Metadata{Host: "late.invalid"})
		if c != nil {
			c.Close()
			done <- errors.New("returned a late connection")
			return
		}
		done <- err
	}()
	<-entered
	wrapped.Close()
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	defer server.Close()
	server.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, err := server.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("late stream did not close: %v", err)
	}
}
