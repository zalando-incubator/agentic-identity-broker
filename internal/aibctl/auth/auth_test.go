package auth_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/auth"
)

// TestHelperProcess is re-executed as the token command under test.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("AIBCTL_HELPER_PROCESS") != "1" {
		return
	}
	switch os.Getenv("AIBCTL_HELPER_MODE") {
	case "ok":
		_, _ = fmt.Fprintln(os.Stdout, "  tok-123  ")
	case "fail":
		_, _ = fmt.Fprintln(os.Stderr, "login required")
		os.Exit(7)
	case "empty":
	case "sleep":
		time.Sleep(5 * time.Second)
	}
	os.Exit(0)
}

func helperArgv(mode string) []string {
	return []string{os.Args[0], "-test.run=^TestHelperProcess$", "--", mode}
}

func helperRunner(mode string) auth.Runner {
	return func(ctx context.Context, argv []string) ([]byte, []byte, error) {
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) // #nosec G204 -- test helper re-exec.
		cmd.Env = append(os.Environ(), "AIBCTL_HELPER_PROCESS=1", "AIBCTL_HELPER_MODE="+mode)
		var stdout, stderr strings.Builder
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		return []byte(stdout.String()), []byte(stderr.String()), err
	}
}

func TestCommandToken(t *testing.T) {
	t.Run("trims stdout", func(t *testing.T) {
		src := auth.CommandToken{Argv: helperArgv("ok"), Timeout: 5 * time.Second, Runner: helperRunner("ok")}
		tok, err := src.Token(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "tok-123", tok)
	})
	t.Run("non-zero exit reports stderr", func(t *testing.T) {
		src := auth.CommandToken{Argv: helperArgv("fail"), Timeout: 5 * time.Second, Runner: helperRunner("fail")}
		_, err := src.Token(context.Background())
		require.Error(t, err)
		var authErr *auth.Error
		require.ErrorAs(t, err, &authErr)
		assert.Contains(t, err.Error(), "exit status 7")
		assert.Contains(t, err.Error(), "login required")
	})
	t.Run("empty output", func(t *testing.T) {
		src := auth.CommandToken{Argv: helperArgv("empty"), Timeout: 5 * time.Second, Runner: helperRunner("empty")}
		_, err := src.Token(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no output")
	})
	t.Run("timeout", func(t *testing.T) {
		src := auth.CommandToken{Argv: helperArgv("sleep"), Timeout: 100 * time.Millisecond, Runner: helperRunner("sleep")}
		_, err := src.Token(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")
	})
	t.Run("command not found uses default runner", func(t *testing.T) {
		src := auth.CommandToken{Argv: []string{"aibctl-definitely-missing-binary"}, Timeout: time.Second}
		_, err := src.Token(context.Background())
		require.Error(t, err)
		var authErr *auth.Error
		assert.ErrorAs(t, err, &authErr)
		assert.Contains(t, err.Error(), "aibctl-definitely-missing-binary")
	})
	t.Run("empty argv", func(t *testing.T) {
		src := auth.CommandToken{}
		_, err := src.Token(context.Background())
		assert.Error(t, err)
	})
}

type countingSource struct {
	calls int
	tok   string
	err   error
}

func (c *countingSource) Token(context.Context) (string, error) {
	c.calls++
	return c.tok, c.err
}

func TestAuthenticatorApply(t *testing.T) {
	newReq := func() *http.Request {
		req, err := http.NewRequest(http.MethodGet, "http://example/api", nil)
		require.NoError(t, err)
		return req
	}

	t.Run("bearer and principal", func(t *testing.T) {
		src := &countingSource{tok: "abc"}
		a := auth.New(src, "X-Remote-User", "admin@example.com")
		req := newReq()
		require.NoError(t, a.Apply(context.Background(), req))
		assert.Equal(t, "Bearer abc", req.Header.Get("Authorization"))
		assert.Equal(t, "admin@example.com", req.Header.Get("X-Remote-User"))
	})
	t.Run("principal only", func(t *testing.T) {
		a := auth.New(nil, "X-Admin", "me")
		req := newReq()
		require.NoError(t, a.Apply(context.Background(), req))
		assert.Empty(t, req.Header.Get("Authorization"))
		assert.Equal(t, "me", req.Header.Get("X-Admin"))
	})
	t.Run("token is memoised per process", func(t *testing.T) {
		src := &countingSource{tok: "abc"}
		a := auth.New(src, "", "")
		require.NoError(t, a.Apply(context.Background(), newReq()))
		require.NoError(t, a.Apply(context.Background(), newReq()))
		assert.Equal(t, 1, src.calls)
	})
	t.Run("token errors propagate", func(t *testing.T) {
		src := &countingSource{err: errors.New("boom")}
		a := auth.New(src, "", "")
		err := a.Apply(context.Background(), newReq())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})
	t.Run("static token", func(t *testing.T) {
		a := auth.New(auth.StaticToken("s"), "", "")
		req := newReq()
		require.NoError(t, a.Apply(context.Background(), req))
		assert.Equal(t, "Bearer s", req.Header.Get("Authorization"))
	})
}
