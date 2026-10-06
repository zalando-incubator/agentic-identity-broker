package handlers

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SPAHandler serves a Single Page Application with History API fallback.
// Routes starting with /api/ are ignored (handled by API routes).
// All other routes serve the SPA's index.html to enable client-side routing.
type SPAHandler struct {
	staticPath            string
	logger                *slog.Logger
	contentSecurityPolicy string
	agentDecisionPreloads string
	approvalPreloads      string
}

// NewSPAHandler creates a new SPA handler.
// staticPath: path to the directory containing the SPA files (e.g., "./dist")
func NewSPAHandler(staticPath string, logger *slog.Logger) *SPAHandler {
	if logger == nil {
		logger = slog.Default()
	}
	const resourcePolicy = "font-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
	policy := "script-src 'self'; " + resourcePolicy
	manifestPath := filepath.Join(staticPath, "csp-hashes.json")
	hash, err := readSPAScriptHash(manifestPath)
	if err != nil {
		logger.Error("failed to load SPA script hash", "path", manifestPath, "error", err)
	} else {
		policy = "script-src 'self' " + hash + "; " + resourcePolicy
	}
	agentDecisionPreloads, approvalPreloads := readSPAPreloads(staticPath)
	return &SPAHandler{
		staticPath:            staticPath,
		logger:                logger,
		contentSecurityPolicy: policy,
		agentDecisionPreloads: agentDecisionPreloads,
		approvalPreloads:      approvalPreloads,
	}
}

// ServeHTTP implements http.Handler interface.
// Serves static files with History API fallback for client-side routing.
func (h *SPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", h.contentSecurityPolicy)

	// API routes should not be handled by SPA
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}

	filePath := strings.TrimPrefix(r.URL.Path, "/")
	for _, segment := range strings.Split(filePath, "/") {
		if segment == ".." {
			http.NotFound(w, r)
			return
		}
	}
	path := filepath.Join(h.staticPath, filePath)

	// Check if the file exists
	fileInfo, err := os.Stat(path) // #nosec G703 -- request path rejects parent segments before static-root resolution.

	// If the file doesn't exist or is a directory, serve index.html (History API fallback)
	if err != nil || fileInfo.IsDir() {
		// History API fallback: serve the SPA entrypoint for client-side routes.
		indexPath := filepath.Join(h.staticPath, "index.html")

		// Check if index.html exists
		if _, err := os.Stat(indexPath); err != nil {
			h.logger.Error("SPA index.html not found",
				"path", indexPath,
				"error", err)
			http.Error(w, "SPA not configured", http.StatusNotFound)
			return
		}

		if links := h.decisionPreloads(r); links != "" {
			w.Header().Set("Link", links)
		}
		h.serveStaticFile(w, r, indexPath)
		return
	}

	// File exists, serve it directly
	h.serveStaticFile(w, r, path)
}

// serveStaticFile negotiates build-produced representations, never compressing a response.
func (h *SPAHandler) serveStaticFile(w http.ResponseWriter, r *http.Request, path string) {
	// ServeFile canonicalizes index.html URLs before serving a body. Do not attach
	// a compressed representation's headers to that uncompressed redirect.
	if strings.HasSuffix(r.URL.Path, "/index.html") {
		http.ServeFile(w, r, path) // #nosec G703 -- original path passed traversal checks.
		return
	}
	w.Header().Add("Vary", "Accept-Encoding")
	qualities := spaEncodingQualities(r.Header.Values("Accept-Encoding"))
	candidates := [2]struct {
		encoding string
		suffix   string
		quality  int
	}{
		{encoding: "br", suffix: ".br", quality: qualities.brotli},
		{encoding: "gzip", suffix: ".gz", quality: qualities.gzip},
	}
	if candidates[1].quality > candidates[0].quality {
		candidates[0], candidates[1] = candidates[1], candidates[0]
	}
	for _, candidate := range candidates {
		if candidate.quality <= 0 || candidate.quality < qualities.identity {
			continue
		}
		compressedPath := path + candidate.suffix
		info, err := os.Stat(compressedPath) // #nosec G703 -- suffix is fixed and original path passed traversal checks.
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		contentType := mime.TypeByExtension(filepath.Ext(path))
		if contentType == "" {
			// Preserve ServeFile's original-byte sniffing for extensions unknown to mime.
			relativePath, err := filepath.Rel(h.staticPath, path)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			original, err := os.OpenInRoot(h.staticPath, relativePath)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			var sample [512]byte
			n, readErr := original.Read(sample[:])
			_ = original.Close()
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				http.Error(w, "Unable to read static representation", http.StatusInternalServerError)
				return
			}
			contentType = http.DetectContentType(sample[:n])
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Encoding", candidate.encoding)
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
		http.ServeFile(w, r, compressedPath) // #nosec G703 -- fixed companion suffix on the checked static path.
		return
	}
	if qualities.identity == 0 {
		http.Error(w, "No acceptable static representation", http.StatusNotAcceptable)
		return
	}
	http.ServeFile(w, r, path) // #nosec G703 -- original path passed traversal checks.
}

type spaEncodingPreference struct {
	brotli   int
	gzip     int
	identity int
}

func spaEncodingQualities(headers []string) spaEncodingPreference {
	preference := spaEncodingPreference{brotli: -1, gzip: -1, identity: -1}
	wildcard := -1
	for _, header := range headers {
		for item := range strings.SplitSeq(header, ",") {
			coding, parameter, hasParameter := strings.Cut(strings.TrimSpace(item), ";")
			quality := 1000
			if hasParameter {
				name, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
				if !ok || !strings.EqualFold(strings.TrimSpace(name), "q") {
					quality = 0
				} else {
					quality = spaEncodingQuality(strings.TrimSpace(value))
				}
			}
			switch {
			case strings.EqualFold(strings.TrimSpace(coding), "br"):
				preference.brotli = quality
			case strings.EqualFold(strings.TrimSpace(coding), "gzip"):
				preference.gzip = quality
			case strings.EqualFold(strings.TrimSpace(coding), "identity"):
				preference.identity = quality
			case strings.TrimSpace(coding) == "*":
				wildcard = quality
			}
		}
	}
	if preference.brotli == -1 {
		preference.brotli = wildcard
	}
	if preference.gzip == -1 {
		preference.gzip = wildcard
	}
	// An implicit identity is acceptable, but does not override an explicit
	// preference for compression. Wildcard q=0 excludes implicit identity.
	if preference.identity == -1 && wildcard == 0 {
		preference.identity = 0
	}
	return preference
}

// HTTP qvalues have at most three decimal places and range from zero to one.
func spaEncodingQuality(value string) int {
	whole, fraction, decimal := strings.Cut(value, ".")
	if whole != "0" && whole != "1" || len(fraction) > 3 {
		return 0
	}
	quality := 0
	if whole == "1" {
		quality = 1000
	}
	if !decimal {
		return quality
	}
	scale := 100
	for i := range len(fraction) {
		digit := fraction[i]
		if digit < '0' || digit > '9' || whole == "1" && digit != '0' {
			return 0
		}
		quality += int(digit-'0') * scale
		scale /= 10
	}
	return quality
}

type spaManifestChunk struct {
	File    string   `json:"file"`
	Imports []string `json:"imports"`
	CSS     []string `json:"css"`
}

func readSPAPreloads(staticPath string) (agentDecision, approval string) {
	root, err := os.OpenRoot(staticPath)
	if err != nil {
		return "", ""
	}
	defer func() { _ = root.Close() }()
	data, err := root.ReadFile(".vite/manifest.json")
	if err != nil {
		return "", ""
	}
	var manifest map[string]spaManifestChunk
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", ""
	}
	return spaPreloadGraph(manifest, "src/pages/AgentDecisionPage.tsx"),
		spaPreloadGraph(manifest, "src/pages/ApprovalPage.tsx")
}

func spaPreloadGraph(manifest map[string]spaManifestChunk, route string) string {
	visited := make(map[string]bool)
	assets := make(map[string]bool)
	var links []string
	addAsset := func(path string, stylesheet bool) bool {
		if !validSPAPreloadPath(path) {
			return false
		}
		var attributes string
		switch {
		case strings.HasSuffix(path, ".css"):
			attributes = "; rel=preload; as=style; crossorigin"
		case !stylesheet && strings.HasSuffix(path, ".js"):
			attributes = "; rel=modulepreload; crossorigin"
		default:
			return false
		}
		if !assets[path] {
			assets[path] = true
			links = append(links, "</"+path+">"+attributes)
		}
		return true
	}
	var visit func(string) bool
	visit = func(key string) bool {
		if visited[key] {
			return true
		}
		chunk, ok := manifest[key]
		if !ok || !addAsset(chunk.File, false) {
			return false
		}
		visited[key] = true
		for _, stylesheet := range chunk.CSS {
			if !addAsset(stylesheet, true) {
				return false
			}
		}
		for _, dependency := range chunk.Imports {
			if !visit(dependency) {
				return false
			}
		}
		return true
	}
	// Dynamic imports deliberately remain deferred, including console-only UI.
	if !visit("index.html") || !visit(route) {
		return ""
	}
	return strings.Join(links, ", ")
}

func validSPAPreloadPath(path string) bool {
	if !strings.HasPrefix(path, "assets/") || !fs.ValidPath(path) {
		return false
	}
	for _, char := range path {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' ||
			char == '/' || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func (h *SPAHandler) decisionPreloads(r *http.Request) string {
	route, id, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if !ok || id == "" || strings.Contains(id, "/") {
		return ""
	}
	switch route {
	case "agents":
		if r.URL.Query().Has("session_token") {
			return h.agentDecisionPreloads
		}
	case "approvals":
		return h.approvalPreloads
	}
	return ""
}

func readSPAScriptHash(path string) (string, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	data, err := root.ReadFile(filepath.Base(path))
	if err != nil {
		return "", err
	}
	var manifest struct {
		ScriptSource string `json:"script-src"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", err
	}
	const prefix = "'sha256-"
	hash := manifest.ScriptSource
	if len(hash) != len(prefix)+base64.StdEncoding.EncodedLen(sha256.Size)+1 || !strings.HasPrefix(hash, prefix) || !strings.HasSuffix(hash, "'") {
		return "", errors.New("script-src must contain one quoted SHA-256 hash")
	}
	var digest [sha256.Size + 1]byte
	size, err := base64.StdEncoding.Decode(digest[:], []byte(hash[len(prefix):len(hash)-1]))
	if err != nil || size != sha256.Size {
		return "", errors.New("script-src contains an invalid SHA-256 digest")
	}
	return hash, nil
}
