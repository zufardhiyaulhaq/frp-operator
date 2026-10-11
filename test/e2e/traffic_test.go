//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// eventually retries check until it returns nil or timeout passes; then the last error fails t.
func eventually(t *testing.T, timeout time.Duration, what string, check func(ctx context.Context) error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		err := check(ctx)
		cancel()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: still failing after %s: %v", what, timeout, err)
		}
		time.Sleep(2 * time.Second)
	}
}

func expectTCPEcho(t *testing.T, host string, port int) { t.Helper(); expectEcho(t, "TCP", host, port) }
func expectUDPEcho(t *testing.T, host string, port int) { t.Helper(); expectEcho(t, "UDP", host, port) }

// expectEcho sends a unique line from the test client to host:port and expects it back.
// stdin stays open for 2s after the line: frp closes both directions when the client
// half-closes, so sending EOF right away would drop the echo before it comes back.
func expectEcho(t *testing.T, proto, host string, port int) {
	t.Helper()
	token := fmt.Sprintf("ping-%d", time.Now().UnixNano())
	eventually(t, 30*time.Second, fmt.Sprintf("%s echo via %s:%d", proto, host, port), func(ctx context.Context) error {
		out, err := env.sh(ctx, fmt.Sprintf("(echo %s; sleep 2) | socat -t 3 - %s:%s:%d", token, proto, host, port))
		if err != nil {
			return err
		}
		if !strings.Contains(out, token) {
			return fmt.Errorf("got %q, want %q echoed back", out, token)
		}
		return nil
	})
}

// expectNoTCP checks three times over ~10s that nothing answers on host:port.
func expectNoTCP(t *testing.T, host string, port int) {
	t.Helper()
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := env.sh(ctx, fmt.Sprintf("echo probe | socat -t 2 - TCP:%s:%d,connect-timeout=2", host, port))
		cancel()
		if err == nil {
			t.Fatalf("TCP %s:%d answered (%q), want nothing listening", host, port, out)
		}
		time.Sleep(3 * time.Second)
	}
}

// curl runs curl on the test client and returns stdout.
func curl(ctx context.Context, args string) (string, error) {
	return env.sh(ctx, "curl -sS --max-time 5 "+args)
}
