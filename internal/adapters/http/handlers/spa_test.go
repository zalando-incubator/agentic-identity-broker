package handlers

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSPAHandlerServesAssetsWithoutEscapingStaticPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	staticPath := filepath.Join(root, "dist")
	require.NoError(t, os.MkdirAll(filepath.Join(staticPath, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staticPath, "index.html"), []byte("index"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(staticPath, "assets", "app.js"), []byte("asset"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret"), 0o600))

	handler := NewSPAHandler(staticPath, slog.New(slog.NewTextHandler(io.Discard, nil)))

	t.Run("serves a root-mounted asset", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "asset", recorder.Body.String())
	})

	t.Run("does not serve a file outside the static path", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/%2e%2e/secret.txt", nil))

		require.Equal(t, http.StatusNotFound, recorder.Code)
	})
}

func TestSPAHandlerSetsSecurityHeaders(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	staticPath := filepath.Join(root, "dist")
	require.NoError(t, os.MkdirAll(filepath.Join(staticPath, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staticPath, "index.html"), []byte("index"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(staticPath, "assets", "app.js"), []byte("asset"), 0o600))

	handler := NewSPAHandler(staticPath, slog.New(slog.NewTextHandler(io.Discard, nil)))
	securityHeaders := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
	}

	for _, test := range []struct {
		name string
		path string
	}{
		{name: "serves a static asset", path: "/assets/app.js"},
		{name: "falls back to the SPA entrypoint", path: "/delegations"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))

			require.Equal(t, http.StatusOK, recorder.Code)
			for header, expected := range securityHeaders {
				require.Equal(t, []string{expected}, recorder.Header().Values(header), header)
			}
		})
	}
}

func TestSPAHandlerRejectsEscapingHashManifest(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	staticPath := filepath.Join(root, "dist")
	require.NoError(t, os.Mkdir(staticPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(staticPath, "index.html"), []byte("index"), 0o600))
	digest := sha256.Sum256([]byte("theme script"))
	hash := "'sha256-" + base64.StdEncoding.EncodeToString(digest[:]) + "'"
	manifest := filepath.Join(root, "outside.json")
	require.NoError(t, os.WriteFile(manifest, []byte(`{"script-src":"`+hash+`"}`), 0o600))
	require.NoError(t, os.Symlink(manifest, filepath.Join(staticPath, "csp-hashes.json")))
	handler := NewSPAHandler(staticPath, slog.New(slog.NewTextHandler(io.Discard, nil)))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/settings", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotContains(t, recorder.Header().Get("Content-Security-Policy"), hash)
}

func TestSPAHandlerThemeCSP(t *testing.T) {
	t.Parallel()
	digest := sha256.Sum256([]byte("document.documentElement.dataset.theme = 'dark'"))
	hash := "'sha256-" + base64.StdEncoding.EncodeToString(digest[:]) + "'"
	for _, tc := range []struct {
		name          string
		manifest      string
		writeManifest bool
		valid         bool
	}{
		{name: "valid build hash", manifest: `{"script-src":"` + hash + `"}`, writeManifest: true, valid: true},
		{name: "missing build manifest"},
		{name: "invalid JSON", manifest: `{`, writeManifest: true},
		{name: "wrong hash type", manifest: `{"script-src":[]}`, writeManifest: true},
		{name: "missing script entry", manifest: `{}`, writeManifest: true},
		{name: "invalid digest", manifest: `{"script-src":"'sha256-YQ=='"}`, writeManifest: true},
		{name: "policy injection", manifest: `{"script-src":"'unsafe-inline'; script-src *"}`, writeManifest: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("<main>Application</main>"), 0o600))
			manifestPath := filepath.Join(root, "csp-hashes.json")
			if tc.writeManifest {
				require.NoError(t, os.WriteFile(manifestPath, []byte(tc.manifest), 0o600))
			}
			var logs bytes.Buffer
			handler := NewSPAHandler(root, slog.New(slog.NewJSONHandler(&logs, nil)))
			if tc.valid {
				require.NoError(t, os.Remove(manifestPath))
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/settings", nil))
			require.Equal(t, http.StatusOK, recorder.Code)
			policy := recorder.Header().Get("Content-Security-Policy")
			for _, directive := range []string{"font-src 'self'", "img-src 'self' data:", "connect-src 'self'", "object-src 'none'", "base-uri 'self'", "frame-ancestors 'none'"} {
				require.Contains(t, policy, directive)
			}
			require.NotContains(t, policy, "'unsafe-inline'")
			require.NotContains(t, policy, "default-src")
			require.NotContains(t, policy, "style-src")
			if tc.valid {
				require.Contains(t, policy, "script-src 'self' "+hash)
			} else {
				require.Contains(t, policy, "script-src 'self';")
				require.NotContains(t, policy, "sha256-")
				var entry map[string]any
				require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
				require.Equal(t, "ERROR", entry["level"])
				require.Equal(t, manifestPath, entry["path"])
			}
			require.Equal(t, "DENY", recorder.Header().Get("X-Frame-Options"))
			require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
			api := httptest.NewRecorder()
			handler.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/api/unhandled", nil))
			require.Equal(t, http.StatusNotFound, api.Code)
		})
	}
}

func TestSPAHandlerPrecompressedRepresentations(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		accept   string
		missing  string
		encoding string
		status   int
	}{
		{name: "prefers Brotli on equal quality", accept: "gzip, br", encoding: "br"},
		{name: "honors gzip preference", accept: "br;q=0.4, gzip;q=0.8", encoding: "gzip"},
		{name: "explicit identity preference", accept: "br;q=0.4, identity;q=1"},
		{name: "Brotli forbidden", accept: "br;q=0, gzip", encoding: "gzip"},
		{name: "both compressed encodings forbidden", accept: "br;q=0, gzip;q=0"},
		{name: "wildcard permits Brotli", accept: "*;q=0.5", encoding: "br"},
		{name: "explicit prohibition overrides wildcard", accept: "br;q=0, *;q=0.5", encoding: "gzip"},
		{name: "wildcard prohibition allows explicit gzip", accept: "*;q=0, gzip;q=0.7", encoding: "gzip"},
		{name: "unsupported encoding", accept: "deflate"},
		{name: "absent encoding"},
		{name: "case insensitive coding", accept: "GZIP", encoding: "gzip"},
		{name: "invalid quality does not enable Brotli", accept: "br;q=wat, gzip;q=0.5", encoding: "gzip"},
		{name: "unavailable Brotli falls back to gzip", accept: "br, gzip", missing: "br", encoding: "gzip"},
		{name: "unavailable companions fall back to identity", accept: "br, gzip", missing: "both"},
		{name: "unacceptable identity and missing companions", accept: "br, gzip, identity;q=0", missing: "both", status: http.StatusNotAcceptable},
		{name: "all representations forbidden", accept: "*;q=0", status: http.StatusNotAcceptable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, resource := range []struct {
				file string
				url  string
				mime string
			}{
				{file: "assets/app.js", url: "/assets/app.js", mime: "javascript"},
				{file: "assets/app.css", url: "/assets/app.css", mime: "text/css"},
				{file: "index.html", url: "/agents/example?session_token=private", mime: "text/html"},
			} {
				t.Run(resource.file, func(t *testing.T) {
					root := t.TempDir()
					require.NoError(t, os.MkdirAll(filepath.Join(root, "assets"), 0o755))
					original := []byte("public asset")
					var compressed bytes.Buffer
					writer := gzip.NewWriter(&compressed)
					_, err := writer.Write(original)
					require.NoError(t, err)
					require.NoError(t, writer.Close())
					brotli, err := base64.StdEncoding.DecodeString("GwsA+CVAihBGSoSQmAM=")
					require.NoError(t, err)
					representations := map[string][]byte{"": original, "gzip": compressed.Bytes(), "br": brotli}
					path := filepath.Join(root, resource.file)
					require.NoError(t, os.WriteFile(path, original, 0o600))
					if tc.missing != "both" {
						require.NoError(t, os.WriteFile(path+".gz", compressed.Bytes(), 0o600))
						if tc.missing != "br" {
							require.NoError(t, os.WriteFile(path+".br", brotli, 0o600))
						}
					}
					handler := NewSPAHandler(root, slog.New(slog.NewTextHandler(io.Discard, nil)))
					for _, method := range []string{http.MethodGet, http.MethodHead} {
						recorder := httptest.NewRecorder()
						recorder.Header().Set("Vary", "Origin")
						request := httptest.NewRequest(method, resource.url, nil)
						request.Header.Set("Accept-Encoding", tc.accept)
						handler.ServeHTTP(recorder, request)
						status := tc.status
						if status == 0 {
							status = http.StatusOK
						}
						require.Equal(t, status, recorder.Code)
						require.Contains(t, strings.Join(recorder.Header().Values("Vary"), ","), "Accept-Encoding")
						require.Contains(t, strings.Join(recorder.Header().Values("Vary"), ","), "Origin")
						require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
						require.Equal(t, "DENY", recorder.Header().Get("X-Frame-Options"))
						require.Contains(t, recorder.Header().Get("Content-Security-Policy"), "script-src 'self';")
						require.Equal(t, tc.encoding, recorder.Header().Get("Content-Encoding"))
						if status != http.StatusOK {
							continue
						}
						require.Contains(t, recorder.Header().Get("Content-Type"), resource.mime)
						require.Equal(t, strconv.Itoa(len(representations[tc.encoding])), recorder.Header().Get("Content-Length"))
						if method == http.MethodHead {
							require.Empty(t, recorder.Body.Bytes())
						} else {
							require.Equal(t, representations[tc.encoding], recorder.Body.Bytes())
						}
					}
				})
			}
		})
	}
}

func TestSPAHandlerPrecompressedAPIAndResourceFallback(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("public asset"), 0o600))
	brotli, err := base64.StdEncoding.DecodeString("GwsA+CVAihBGSoSQmAM=")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html.br"), brotli, 0o600))
	handler := NewSPAHandler(root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, url := range []string{"/api/missing", "/api/agents/example?session_token=secret", "/%2e%2e/secret.txt"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, url, nil)
		request.Header.Set("Accept-Encoding", "br, gzip")
		handler.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusNotFound, recorder.Code)
		require.Empty(t, recorder.Header().Get("Content-Encoding"))
		require.Empty(t, recorder.Header().Get("Link"))
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
	request.Header.Set("Accept-Encoding", "br")
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "br", recorder.Header().Get("Content-Encoding"))
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/html")
	require.Equal(t, brotli, recorder.Body.Bytes())
}

func TestSPAHandlerDecisionPreloads(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".vite"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o600))
	manifest := `{
		"index.html":{"file":"assets/entry.js","isEntry":true,"imports":["shared"],"css":["assets/entry.css"],"dynamicImports":["console"]},
		"shared":{"file":"assets/shared.js","imports":["index.html"],"css":["assets/shared.css"]},
		"src/pages/AgentDecisionPage.tsx":{"file":"assets/decision.js","imports":["shared"],"css":["assets/decision.css"],"dynamicImports":["table","command"]},
		"src/pages/ApprovalPage.tsx":{"file":"assets/approval.js","imports":["shared"],"css":["assets/approval.css"]},
		"console":{"file":"assets/ConsoleLayout.js"},
		"table":{"file":"assets/Table.js"},
		"command":{"file":"assets/Command.js"}
	}`
	manifestPath := filepath.Join(root, ".vite", "manifest.json")
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifest), 0o600))
	handler := NewSPAHandler(root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, os.Remove(manifestPath))
	for _, tc := range []struct {
		url   string
		route string
	}{
		{url: "/agents/example?session_token=private", route: "decision"},
		{url: "/agents/example?session_token=", route: "decision"},
		{url: "/approvals/example", route: "approval"},
		{url: "/agents/example"},
		{url: "/agents/example?other=session_token"},
		{url: "/agents/example/extra?session_token=private"},
		{url: "/approvals"},
		{url: "/delegations"},
		{url: "/settings"},
		{url: "/api/approvals/example"},
	} {
		t.Run(tc.url, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, httptest.NewRequest(method, tc.url, nil))
				links := strings.Join(recorder.Header().Values("Link"), ", ")
				if tc.route == "" {
					require.Empty(t, links)
					continue
				}
				expected := []string{
					`</assets/entry.js>; rel=modulepreload; crossorigin`,
					`</assets/shared.js>; rel=modulepreload; crossorigin`,
					`</assets/` + tc.route + `.js>; rel=modulepreload; crossorigin`,
					`</assets/entry.css>; rel=preload; as=style; crossorigin`,
					`</assets/shared.css>; rel=preload; as=style; crossorigin`,
					`</assets/` + tc.route + `.css>; rel=preload; as=style; crossorigin`,
				}
				require.ElementsMatch(t, expected, strings.Split(links, ", "))
				require.NotContains(t, links, "private")
				require.NotContains(t, links, "session_token")
			}
		})
	}
}

func TestSPAHandlerInvalidPreloadManifest(t *testing.T) {
	t.Parallel()
	for _, manifest := range []string{
		"", "{", "null",
		`{"index.html":{"file":"assets/entry.js"},"src/pages/ApprovalPage.tsx":{"file":"../secret.js"}}`,
		`{"index.html":{"file":"assets/entry.js"},"src/pages/ApprovalPage.tsx":{"file":"/assets/route.js"}}`,
		`{"index.html":{"file":"assets/entry.js"},"src/pages/ApprovalPage.tsx":{"file":"https://evil.test/route.js"}}`,
		`{"index.html":{"file":"assets/entry.js"},"src/pages/ApprovalPage.tsx":{"file":"assets/route.js?secret=yes"}}`,
		`{"index.html":{"file":"assets/entry.js"},"src/pages/ApprovalPage.tsx":{"file":"assets/%2e%2e/route.js"}}`,
		`{"index.html":{"file":"assets/entry.js"},"src/pages/ApprovalPage.tsx":{"file":"assets/route.js\r\nX-Evil: yes"}}`,
		`{"index.html":{"file":"assets/entry.js"},"src/pages/ApprovalPage.tsx":{"file":"assets/route.js","css":["assets/a.css>; rel=preload"]}}`,
		`{"index.html":{"file":"assets/entry.js"},"src/pages/ApprovalPage.tsx":{"file":"assets/route.js","imports":["missing"]}}`,
	} {
		t.Run(manifest, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o600))
			if manifest != "" {
				require.NoError(t, os.MkdirAll(filepath.Join(root, ".vite"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(root, ".vite", "manifest.json"), []byte(manifest), 0o600))
			}
			handler := NewSPAHandler(root, slog.New(slog.NewTextHandler(io.Discard, nil)))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/approvals/example", nil))
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, "index", recorder.Body.String())
			require.Empty(t, recorder.Header().Get("Link"))
		})
	}
}

func TestSPAHandlerPrecompressedIndexRedirect(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("public asset"), 0o600))
	brotli, err := base64.StdEncoding.DecodeString("GwsA+CVAihBGSoSQmAM=")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html.br"), brotli, 0o600))
	handler := NewSPAHandler(root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/index.html?theme=dark", nil)
	request.Header.Set("Accept-Encoding", "br")
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusMovedPermanently, recorder.Code)
	require.Equal(t, "./?theme=dark", recorder.Header().Get("Location"))
	require.Empty(t, recorder.Header().Get("Content-Encoding"))
	require.Empty(t, recorder.Header().Get("Content-Length"))
}
