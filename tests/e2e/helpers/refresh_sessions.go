package helpers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/jmoiron/sqlx"
)

type RefreshTokenResult struct {
	Status   int
	Tokens   ports.TokenResponse
	Error    string
	ErrorURI string
	Body     map[string]any
}

type RefreshAuthorization struct {
	Code     string
	Verifier string
	Tokens   ports.TokenResponse
}

func AuthorizeRefreshSession(ctx context.Context, baseURL string, client fixtures.RefreshClient, scope string) (*RefreshAuthorization, error) {
	verifier := PKCEVerifier()
	redirectURI := client.Agent.RedirectURIs[0]
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {client.Agent.ID.String()},
		"redirect_uri":          {redirectURI},
		"scope":                 {scope},
		"state":                 {"refresh-session-acceptance"},
		"code_challenge":        {GenerateCodeChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/oauth2/authorize?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-Remote-User", client.Principal.String())
	transport := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := transport.Do(request)
	if err != nil {
		return nil, fmt.Errorf("authorize refresh session: %w", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusFound {
		return nil, fmt.Errorf("authorization returned status %d", response.StatusCode)
	}
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		return nil, fmt.Errorf("parse authorization redirect: %w", err)
	}
	code := location.Query().Get("code")
	if code == "" {
		return nil, fmt.Errorf("authorization redirect contains no code")
	}
	result, err := ExchangeRefreshCode(ctx, baseURL, client, code, verifier)
	if err != nil {
		return nil, err
	}
	if result.Status != http.StatusOK {
		return nil, fmt.Errorf("authorization-code exchange returned status %d, error %s", result.Status, result.Error)
	}
	if result.Tokens.AccessToken == "" || result.Tokens.RefreshToken == "" {
		return nil, fmt.Errorf("authorization-code exchange returned no token pair")
	}
	return &RefreshAuthorization{Code: code, Verifier: verifier, Tokens: result.Tokens}, nil
}

func ExchangeRefreshCode(ctx context.Context, baseURL string, client fixtures.RefreshClient, code, verifier string) (*RefreshTokenResult, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {client.Agent.ID.String()},
		"code":          {code},
		"redirect_uri":  {client.Agent.RedirectURIs[0]},
		"code_verifier": {verifier},
	}
	if client.Secret != "" {
		form.Set("client_secret", client.Secret)
	}
	return requestRefreshTokens(ctx, baseURL, form)
}

func RotateRefreshSession(ctx context.Context, baseURL string, client fixtures.RefreshClient, token, scope string) (*RefreshTokenResult, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {client.Agent.ID.String()},
		"refresh_token": {token},
	}
	if client.Secret != "" {
		form.Set("client_secret", client.Secret)
	}
	if scope != "" {
		form.Set("scope", scope)
	}
	return requestRefreshTokens(ctx, baseURL, form)
}

func requestRefreshTokens(ctx context.Context, baseURL string, form url.Values) (*RefreshTokenResult, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := HTTPClient().Do(request)
	if err != nil {
		return nil, fmt.Errorf("request refresh tokens: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode refresh response: %w", err)
	}
	accessToken, _ := body["access_token"].(string)
	refreshToken, _ := body["refresh_token"].(string)
	tokenType, _ := body["token_type"].(string)
	expiresIn, _ := body["expires_in"].(float64)
	scope, _ := body["scope"].(string)
	errorCode, _ := body["error"].(string)
	errorURI, _ := body["error_uri"].(string)
	return &RefreshTokenResult{
		Status: response.StatusCode,
		Tokens: ports.TokenResponse{
			AccessToken: accessToken, RefreshToken: refreshToken,
			TokenType: tokenType, ExpiresIn: int64(expiresIn), Scope: scope,
		},
		Error: errorCode, ErrorURI: errorURI, Body: body,
	}, nil
}

func RefreshSignature(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func RefreshSharedTime(ctx context.Context, database *sqlx.DB) (time.Time, error) {
	var at time.Time
	err := database.GetContext(ctx, &at, "SELECT clock_timestamp()")
	return at, err
}

func ReadRefreshRoot(ctx context.Context, database *sqlx.DB, token string) (map[string]any, error) {
	var encoded []byte
	if err := database.GetContext(ctx, &encoded, `
		SELECT row_to_json(root) FROM refresh_sessions root
		JOIN refresh_tokens token ON token.session_id = root.id
		WHERE token.signature = $1`, RefreshSignature(token)); err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(encoded, &root); err != nil {
		return nil, err
	}
	return root, nil
}

func LockLegacyRefresh(ctx context.Context, database *sqlx.DB, token string) (*sqlx.Tx, error) {
	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	var signature string
	if err := tx.GetContext(ctx, &signature, `
		SELECT signature FROM refresh_token_sessions
		WHERE signature = $1 FOR UPDATE`, RefreshSignature(token)); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func AgeInitialRefreshSession(ctx context.Context, database *sqlx.DB, token string, age time.Duration) error {
	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var proofInstalled bool
	if err := tx.GetContext(ctx, &proofInstalled, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'refresh_sessions' AND column_name = 'lineage_valid')`); err != nil {
		return err
	}
	if proofInstalled {
		return fmt.Errorf("aging requires an isolated pre-proof refresh fixture")
	}
	seconds := age.Seconds()
	var hasRoots bool
	if err := tx.GetContext(ctx, &hasRoots, "SELECT to_regclass('refresh_sessions') IS NOT NULL"); err != nil {
		return err
	}
	if !hasRoots {
		result, err := tx.ExecContext(ctx, `
			UPDATE refresh_token_sessions SET created_at = created_at - make_interval(secs => $2),
			expires_at = expires_at - make_interval(secs => $2)
			WHERE signature = $1 AND used_at IS NULL`, RefreshSignature(token), seconds)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return fmt.Errorf("initial legacy refresh authority is not unused")
		}
		return tx.Commit()
	}
	var sessionID string
	if err := tx.GetContext(ctx, &sessionID, `
		SELECT root.id FROM refresh_sessions root
		JOIN refresh_tokens token ON token.session_id = root.id
		WHERE token.signature = $1 AND root.previous_signature IS NULL
		AND token.used_at IS NULL FOR UPDATE OF root, token`, RefreshSignature(token)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE refresh_sessions SET
		started_at = started_at - make_interval(secs => $2),
		last_fresh_at = last_fresh_at - make_interval(secs => $2),
		absolute_expires_at = absolute_expires_at - make_interval(secs => $2),
		inactivity_expires_at = inactivity_expires_at - make_interval(secs => $2),
		retain_until = retain_until - make_interval(secs => $2)
		WHERE id = $1`, sessionID, seconds); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE refresh_tokens SET issued_at = issued_at - make_interval(secs => $2),
		expires_at = expires_at - make_interval(secs => $2)
		WHERE session_id = $1`, sessionID, seconds); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE refresh_token_sessions SET created_at = created_at - make_interval(secs => $2),
		expires_at = expires_at - make_interval(secs => $2)
		WHERE session_id = $1`, sessionID, seconds); err != nil {
		return err
	}
	return tx.Commit()
}
