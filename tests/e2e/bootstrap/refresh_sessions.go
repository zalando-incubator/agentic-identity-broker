package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	integrationbootstrap "github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/golang-migrate/migrate/v4"
	"github.com/jmoiron/sqlx"
)

type RefreshEnvironment struct {
	Config  *ports.Config
	Storage *storageadapter.Adapter
	App     *app.App
	Admin   *TestServer
	Enduser *TestServer
}

func NewRefreshEnvironment(cfg *ports.Config, store *storageadapter.Adapter) (*RefreshEnvironment, error) {
	if store == nil {
		var err error
		store, err = NewStorageFactory(TestLogger(slog.LevelInfo)).NewTestStorage()
		if err != nil {
			return nil, err
		}
	}
	environment := &RefreshEnvironment{Config: cfg, Storage: store}
	if err := environment.Start(); err != nil {
		return nil, err
	}
	return environment, nil
}

func (e *RefreshEnvironment) Start() error {
	built, err := NewServerFactory(e.Config, TestLogger(slog.LevelInfo)).BuildApp(e.Storage)
	if err != nil {
		return err
	}
	e.App = built
	e.Admin, err = NewAdminTestServer(built, TestLogger(slog.LevelInfo))
	if err != nil {
		return err
	}
	e.Enduser, err = NewEndUserTestServer(built, TestLogger(slog.LevelInfo))
	if err != nil {
		e.Admin.Close()
		return err
	}
	return helpers.ProvisionSigningKey(e.Admin.BaseURL())
}

func (e *RefreshEnvironment) Stop() {
	if e.Enduser != nil {
		e.Enduser.Close()
		e.Enduser = nil
	}
	if e.Admin != nil {
		e.Admin.Close()
		e.Admin = nil
	}
}

func (e *RefreshEnvironment) Close() {
	e.Stop()
	_ = e.Storage.Close(context.Background())
}

func (e *RefreshEnvironment) AddClient(ctx context.Context, principal id.Principal, confidential bool) (*fixtures.RefreshClient, error) {
	agent := fixtures.RefreshAgent()
	if err := e.Storage.Agents().Create(ctx, agent); err != nil {
		return nil, err
	}
	grant := fixtures.RefreshGrant(principal, agent.ID)
	if err := e.Storage.UserGrants().Create(ctx, grant); err != nil {
		return nil, err
	}
	client := &fixtures.RefreshClient{Agent: agent, Grant: grant, Principal: principal}
	if confidential {
		if err := e.ReplaceCredentials(ctx, client); err != nil {
			return nil, err
		}
	}
	return client, nil
}

func (e *RefreshEnvironment) ReplaceCredentials(ctx context.Context, client *fixtures.RefreshClient) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, e.Admin.BaseURL()+"/api/agents/"+client.Agent.ID.String()+"/client-credentials", nil)
	if err != nil {
		return err
	}
	request.Header.Set("X-Remote-User", fixtures.AdminPrincipal().String())
	response, err := helpers.HTTPClient().Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return fmt.Errorf("credential generation returned status %d", response.StatusCode)
	}
	var credentials struct {
		Secret string `json:"client_secret"`
	}
	if err := json.NewDecoder(response.Body).Decode(&credentials); err != nil {
		return err
	}
	if credentials.Secret == "" {
		return fmt.Errorf("credential generation returned no secret")
	}
	client.Secret = credentials.Secret
	return nil
}

func NewRefreshPostgresStorage(t *testing.T, config *ports.Config) (*storageadapter.Adapter, *sqlx.DB, func(), error) {
	return newRefreshPostgresStorage(t, config, 0)
}

func newRefreshPostgresStorage(t *testing.T, config *ports.Config, version uint) (*storageadapter.Adapter, *sqlx.DB, func(), error) {
	t.Helper()
	shared := integrationbootstrap.RequireSharedPostgres(t)
	root, err := integrationbootstrap.FindProjectRoot()
	if err != nil {
		return nil, nil, nil, err
	}
	template := fmt.Sprintf("refresh_sessions_%d", version)
	_, connectionURL, drop := shared.SetupDatabaseFromTemplate(t, template, func(t *testing.T, database string) {
		runner, err := migrate.New("file://"+filepath.Join(root, "migrations"), shared.ConnectionString(database))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _, _ = runner.Close() }()
		if version == 0 {
			err = runner.Up()
		} else {
			err = runner.Migrate(version)
		}
		if err != nil && err != migrate.ErrNoChange {
			t.Fatal(err)
		}
	})
	cfg := &ports.StorageConfig{
		Backend:  "postgres",
		Postgres: ports.PostgresConfig{ConnectionURL: connectionURL},
		Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
	}
	config.Storage = *cfg
	store, err := storageadapter.NewAdapter(cfg)
	if err != nil {
		drop()
		return nil, nil, nil, err
	}
	database, err := sqlx.Connect("pgx", connectionURL)
	if err != nil {
		_ = store.Close(context.Background())
		drop()
		return nil, nil, nil, err
	}
	cleanup := func() {
		_ = store.Close(context.Background())
		_ = database.Close()
		drop()
	}
	return store, database, cleanup, nil
}

// Network I/O keeps virtual node time fixed while PostgreSQL supplies real time.
// Use one at a time: httptest shutdown closes idle connections across transports.
func NewClockSkewedRefreshEnvironment(t *testing.T, cfg *ports.Config, offset time.Duration) (*RefreshEnvironment, time.Time, func(), error) {
	t.Helper()
	nodeTime := time.Now().UTC().Add(offset)
	type startup struct {
		environment *RefreshEnvironment
		nodeTime    time.Time
		err         error
	}
	started := make(chan startup, 1)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		synctest.Test(t, func(*testing.T) {
			time.Sleep(time.Until(nodeTime))
			store, err := storageadapter.NewAdapter(&cfg.Storage)
			if err != nil {
				started <- startup{err: err}
				return
			}
			environment, err := NewRefreshEnvironment(cfg, store)
			if err != nil {
				_ = store.Close(context.Background())
				started <- startup{err: err}
				return
			}
			started <- startup{environment: environment, nodeTime: time.Now().UTC()}
			<-stop
			environment.Close()
		})
	}()
	result := <-started
	shutdown := func() {
		close(stop)
		<-done
	}
	return result.environment, result.nodeTime, shutdown, result.err
}
