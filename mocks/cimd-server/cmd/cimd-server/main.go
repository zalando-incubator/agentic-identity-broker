// Package main runs the mock CIMD (Client ID Metadata Document) server for Docker Compose.
// It generates a self-signed TLS certificate at startup and serves a CIMD JSON document
// at a configurable path over HTTPS on port 443 (mapped from an internal port).
//
// The document's client_id matches the request URL so the broker's ParseDocument
// validation passes. redirect_uris are loaded from CIMD_REDIRECT_URIS (comma-separated).
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	host := envOrDefault("CIMD_HOST", "cimd-mock")
	port := envOrDefault("CIMD_PORT", "8443")
	docPath := envOrDefault("CIMD_DOC_PATH", "/oauth/client-metadata.json")
	redirectURIs := strings.Split(envOrDefault("CIMD_REDIRECT_URIS", "http://localhost:9002/oauth2/callback"), ",")
	clientName := envOrDefault("CIMD_CLIENT_NAME", "CIMD Demo Agent")

	// The public URL is the URL registered in the broker as client_uri.
	// Since the broker uses this as the fetch URL, client_id in the doc must match it exactly.
	publicURL := fmt.Sprintf("https://%s%s", host, docPath)

	cert, certPEM, err := generateSelfSignedCert(host)
	if err != nil {
		logger.Error("failed to generate TLS certificate", "error", err)
		os.Exit(1)
	}

	// Write CA cert to a well-known path so the broker container can trust it.
	caCertPath := envOrDefault("CIMD_CA_CERT_PATH", "/certs/cimd-ca.crt")
	if err := os.MkdirAll(dirOf(caCertPath), 0o755); err != nil { // #nosec G301 -- generated CA certificate directory is intentionally readable by TLS clients.
		logger.Error("failed to create cert dir", "error", err)
		os.Exit(1)
	}
	if err := os.WriteFile(caCertPath, certPEM, 0o644); err != nil { // #nosec G306 -- CA certificate is public trust material, not a private key.
		logger.Error("failed to write CA cert", "error", err)
		os.Exit(1)
	}
	logger.Info("CA cert written", "path", caCertPath)

	doc := map[string]interface{}{
		"client_id":                  publicURL,
		"client_name":                clientName,
		"redirect_uris":              redirectURIs,
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok"}`)
	})
	mux.HandleFunc(docPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=300")
		if err := json.NewEncoder(w).Encode(doc); err != nil {
			logger.Error("failed to encode CIMD document", "error", err)
		}
	})

	addr := net.JoinHostPort("0.0.0.0", port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		},
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	logger.Info("Starting mock CIMD server",
		"addr", addr,
		"public_url", publicURL,
		"redirect_uris", redirectURIs,
	)

	if err := srv.ListenAndServeTLS("", ""); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func generateSelfSignedCert(host string) (tls.Certificate, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: host},
		DNSNames:              []string{host},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	return tlsCert, certPEM, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func dirOf(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return "."
	}
	return path[:idx]
}
