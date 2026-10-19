// Package aibctl_test is the E2E acceptance suite for the aibctl admin CLI.
// Each It() maps to one acceptance scenario in specs/048-admin-cli/spec.md.
// Commands run in-process through aibctlcmd.Execute against an admin server
// started with the production bootstrap.
package aibctl_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	aibctlcmd "github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/cmd"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

func TestAibctl(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "aibctl Admin CLI E2E Suite")
}

const e2eContextName = "e2e"

type result struct {
	stdout string
	stderr string
	code   int
}

// harness owns one admin server, its storage, and an aibctl config file.
type harness struct {
	server         *bootstrap.TestServer
	storage        *storageadapter.Adapter
	storageFactory *bootstrap.StorageFactory
	configPath     string
}

func newHarness() *harness {
	logger := bootstrap.TestLogger(slog.LevelWarn)
	storageFactory := bootstrap.NewStorageFactory(logger)
	testStorage, err := storageFactory.NewTestStorage()
	Expect(err).NotTo(HaveOccurred())
	application, err := bootstrap.NewServerFactory(fixtures.LocalConfig(), logger).BuildApp(testStorage)
	Expect(err).NotTo(HaveOccurred())
	server, err := bootstrap.NewAdminTestServer(application, logger)
	Expect(err).NotTo(HaveOccurred())

	h := &harness{
		server:         server,
		storage:        testStorage,
		storageFactory: storageFactory,
		configPath:     filepath.Join(GinkgoT().TempDir(), "aibctl", "config.yaml"),
	}
	r := h.run("", "context", "set", e2eContextName, "--server", server.BaseURL(), "--principal", fixtures.AdminPrincipal().Email)
	Expect(r.code).To(Equal(0), r.stderr)
	return h
}

func (h *harness) close() {
	if h.server != nil {
		h.server.Close()
	}
	if h.storage != nil {
		_ = h.storageFactory.CloseStorage(h.storage)
	}
}

// run executes aibctl in-process against the harness config file.
func (h *harness) run(stdin string, args ...string) result {
	var stdout, stderr bytes.Buffer
	code := aibctlcmd.Execute(aibctlcmd.Deps{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader(stdin)}, append([]string{"--config", h.configPath}, args...))
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func (h *harness) mustRun(args ...string) result {
	r := h.run("", args...)
	ExpectWithOffset(1, r.code).To(Equal(0), "stdout: %s\nstderr: %s", r.stdout, r.stderr)
	return r
}

func (h *harness) runJSON(args ...string) map[string]any {
	r := h.mustRun(append(args, "-o", "json")...)
	var out map[string]any
	ExpectWithOffset(1, json.Unmarshal([]byte(r.stdout), &out)).To(Succeed(), r.stdout)
	return out
}

func (h *harness) runJSONList(args ...string) []map[string]any {
	r := h.mustRun(append(args, "-o", "json")...)
	var out []map[string]any
	ExpectWithOffset(1, json.Unmarshal([]byte(r.stdout), &out)).To(Succeed(), r.stdout)
	return out
}

// seedService stores a third-party service directly, which the admin API
// otherwise requires discovery or endpoints for.
func (h *harness) seedService(canonicalID, displayName string) id.ServiceID {
	serviceID := id.NewServiceID()
	canonical := canonicalID
	Expect(h.storage.Services().Create(context.Background(), &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceID,
		CanonicalID: &canonical,
		DisplayName: displayName,
		ClientID:    id.ClientID(canonicalID + "-client"),
		Secret:      fixtures.EncryptedSecret(serviceID.String(), "secret"),
		IssuerURI:   "https://" + canonicalID + ".example.com",
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://" + canonicalID + ".example.com/oauth/token",
			AuthorizeEndpoint: "https://" + canonicalID + ".example.com/oauth/authorize",
		},
		Scopes: []model.OAuthScope{{ScopeValue: "read", Description: "Read"}, {ScopeValue: "write", Description: "Write"}},
	})).To(Succeed())
	return serviceID
}

func tableRows(stdout string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		rows = append(rows, strings.Fields(line))
	}
	return rows
}
