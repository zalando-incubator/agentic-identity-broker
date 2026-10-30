# Quickstart: Portless Redirect URI Registration

**Branch**: `feat/ephemeral-runtime-port` | **Date**: 2026-06-02

## What This Feature Changes

A single new helper function (`MatchesRedirectURI`) and two call-site updates. No migrations, no new config, no API changes.

---

## Step 1 — Write the Unit Tests First (TDD Red Phase)

Add `TestMatchesRedirectURI` to `internal/domain/urivalidation/redirect_test.go` (alongside `TestIsValidRedirectURI`).

```go
func TestMatchesRedirectURI(t *testing.T) {
    cases := []struct {
        name       string
        registered string
        incoming   string
        want       bool
    }{
        // Loopback — port ignored
        {"loopback: portless reg, port in request", "http://localhost/cb", "http://localhost:52341/cb", true},
        {"loopback: explicit port reg, different port", "http://localhost:3000/cb", "http://localhost:9999/cb", true},
        {"loopback: explicit port reg, portless request", "http://localhost:3000/cb", "http://localhost/cb", true},
        {"loopback: both portless", "http://localhost/cb", "http://localhost/cb", true},
        {"loopback: 127.0.0.1 different ports", "http://127.0.0.1:8080/cb", "http://127.0.0.1:51234/cb", true},
        // Loopback — other components must still match
        {"loopback: path mismatch", "http://localhost/cb", "http://localhost:3000/other", false},
        {"loopback: scheme mismatch", "http://localhost/cb", "https://localhost/cb", false},
        {"loopback: host mismatch (localhost vs 127.0.0.1)", "http://localhost/cb", "http://127.0.0.1/cb", false},
        // Non-loopback — exact match required
        {"non-loopback: exact match", "https://app.example.com/cb", "https://app.example.com/cb", true},
        {"non-loopback: port differs", "https://app.example.com/cb", "https://app.example.com:9999/cb", false},
        {"non-loopback: explicit port differs", "https://app.example.com:8443/cb", "https://app.example.com:9000/cb", false},
        {"non-loopback: portless reg, port in request", "https://app.example.com/cb", "https://app.example.com:443/cb", false},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            got := MatchesRedirectURI(tc.registered, tc.incoming)
            if got != tc.want {
                t.Errorf("MatchesRedirectURI(%q, %q) = %v, want %v", tc.registered, tc.incoming, got, tc.want)
            }
        })
    }
}
```

Run `go test ./internal/domain/urivalidation/...` — `TestMatchesRedirectURI` must **fail to compile** (function doesn't exist yet).

---

## Step 2 — Implement `MatchesRedirectURI`

Add to `internal/domain/urivalidation/redirect.go`, below `IsValidRedirectURI`:

```go
// MatchesRedirectURI compares a registered redirect URI against an incoming
// redirect URI. For loopback hosts (localhost, 127.0.0.1) the port component
// is ignored per RFC 8252 §7.3 and OAuth 2.1 §2.3.1; all other components
// must match exactly. For non-loopback hosts all four components (scheme,
// host, port, path+query) must match exactly.
func MatchesRedirectURI(registered, incoming string) bool {
    r, err := url.Parse(registered)
    if err != nil || r.Host == "" {
        return false
    }
    in, err := url.Parse(incoming)
    if err != nil || in.Host == "" {
        return false
    }
    if h := r.Hostname(); h == "localhost" || h == "127.0.0.1" {
        r.Host = h
        in.Host = in.Hostname()
    }
    return r.String() == in.String()
}
```

Run `go test ./internal/domain/urivalidation/...` — tests must now **pass**.

---

## Step 3 — Write E2E Tests (Red Phase)

Create `tests/e2e/cimd_redirect_uri_test.go` covering all US1–US3 acceptance scenarios with realistic `Expect(resp.StatusCode).To(Equal(...))` assertions. See [Testing Strategy in plan.md](plan.md) for the full scenario mapping.

Key pattern (existing cimd tests for reference):
```go
// Scenario US1.1 from specs/20260602-120000-portless-registration/spec.md
It("should accept any ephemeral port for a portless loopback registered URI", func() {
    req := buildAuthorizeRequest(cimdAgent, "http://localhost:52341/callback", state)
    resp, err := client.Do(req)
    Expect(err).NotTo(HaveOccurred())
    // Before fix: 400 invalid_redirect_uri
    // After fix: 302 to consent page
    Expect(resp.StatusCode).To(Equal(http.StatusFound))
    Expect(resp.Header.Get("Location")).To(ContainSubstring("/consent"))
})
```

Run `just test-e2e` — all new `It()` blocks must **fail** with `Expected 400 to equal 302` (or similar).

---

## Step 4 — Wire the Helper into `service.go`

In `internal/domain/oauth2/service.go`, add `urivalidation` import and replace the `==` comparison:

```go
// Before:
for _, allowed := range allowedRedirectURIs {
    if req.RedirectURI == allowed {
        uriAllowed = true
        break
    }
}

// After:
for _, allowed := range allowedRedirectURIs {
    if urivalidation.MatchesRedirectURI(allowed, req.RedirectURI) {
        uriAllowed = true
        break
    }
}
```

Also replace `storage.IsValidRedirectURI` → `urivalidation.IsValidRedirectURI` in the same file.

---

## Step 5 — Wire the Helper into `provider.go`

In `internal/domain/oauth2server/provider.go`, add `urivalidation` import and update the `containsRedirectURI` helper only:

```go
// Before:
func containsRedirectURI(list []string, item string) bool {
    for _, v := range list {
        if v == item {
            return true
        }
    }
    return false
}

// After:
func containsRedirectURI(list []string, item string) bool {
    for _, v := range list {
        if urivalidation.MatchesRedirectURI(v, item) {
            return true
        }
    }
    return false
}
```

Keep `containsScope` as plain string equality so scope validation continues to compare literal scope values.

Also replace `storage.IsValidRedirectURI` → `urivalidation.IsValidRedirectURI` in the same file.

Also update `oauth2/cimd/document.go`: replace the `storage` import with `urivalidation` and update the `storage.IsValidRedirectURI` call.

---

## Step 6 — Verify Green Phase

```bash
just check               # fmt + vet + lint + unit tests
just test-e2e            # all new E2E tests should now pass
```

All 12 E2E scenarios must be green. The existing 028 E2E suite must remain green.

---

## Step 7 — Update ARCHITECTURE.md

Add a note to the redirect URI validation section (or OAuth2 Authorization Server section) documenting the loopback port exception and referencing ADR 028b.

---

## Verification Checklist

```
[ ] go test ./internal/domain/urivalidation/... — PASS (MatchesRedirectURI + IsValidRedirectURI unit tests)
[ ] go test ./internal/domain/oauth2/... — PASS (all existing tests still green)
[ ] go test ./internal/domain/oauth2server/... — PASS
[ ] just check — PASS (fmt, vet, lint, unit tests)
[ ] just test-e2e — all 028b E2E scenarios green
[ ] just test-e2e — all existing 028 E2E scenarios still green (zero regression)
[ ] ARCHITECTURE.md updated
```
