package opa

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var governedDecision = decision("governed", "company.impersonation", "result")

func startManaged(t *testing.T, bootstrap string, admit ports.ImpersonationPolicyAdmission, decisions ...ports.ImpersonationPolicyDecisionRef) ports.ImpersonationPolicySource {
	t.Helper()
	src, err := NewImpersonationPolicySource(ports.ImpersonationOPAEngineConfig{
		Policy: ports.ImpersonationOPAPolicySource{ConfigFile: bootstrap},
	}, decisions, admit, discardLogger())
	require.NoError(t, err)
	t.Cleanup(func() { src.Stop(context.Background()) })
	return src
}

func awaitReady(t *testing.T, src ports.ImpersonationPolicySource, timeout time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return src.AwaitReady(ctx)
}

func evaluate(t *testing.T, rev ports.ImpersonationPolicyRevision) ports.ImpersonationPolicyAction {
	t.Helper()
	action, err := rev.Evaluate(context.Background(), "governed", ports.ImpersonationPolicyInput{})
	require.NoError(t, err)
	return action
}

func TestManagedSourceReadyOnFirstAdmittedSnapshot(t *testing.T) {
	server := newBundleServer(t)
	server.publish(t, "impersonation", "r1", map[string]string{"/p.rego": allowPolicy}, governance(true))
	src := startManaged(t, server.bootstrap(t, "", "impersonation"), requireDecisions("governed"), governedDecision)

	require.NoError(t, awaitReady(t, src, 10*time.Second))
	rev := src.Current()
	require.NotNil(t, rev)
	assert.Equal(t, "impersonation=r1", rev.Label())
	assert.NotEmpty(t, rev.AdmissionID())
	assert.Equal(t, ports.ImpersonationPolicyAllow, evaluate(t, rev))

	server.publish(t, "impersonation", "r2", map[string]string{"/p.rego": allowPolicy}, governance(false))
	require.Eventually(t, func() bool { return src.Current().Label() == "impersonation=r2" }, 10*time.Second, 50*time.Millisecond)
	assert.Equal(t, ports.ImpersonationPolicyDeny, evaluate(t, src.Current()))
	assert.Equal(t, ports.ImpersonationPolicyAllow, evaluate(t, rev), "a held revision is immutable")
	assert.NotEqual(t, rev.AdmissionID(), src.Current().AdmissionID())

	held := src.Current()
	server.publish(t, "impersonation", "r2", map[string]string{"/p.rego": allowPolicy}, governance(true))
	require.Eventually(t, func() bool { return src.Current().AdmissionID() != held.AdmissionID() }, 10*time.Second, 50*time.Millisecond)
	assert.Equal(t, held.Label(), src.Current().Label())
}

func TestManagedSourceKeepsTheActiveRevision(t *testing.T) {
	server := newBundleServer(t)
	server.publish(t, "impersonation", "r1", map[string]string{"/p.rego": allowPolicy}, governance(true))
	src := startManaged(t, server.bootstrap(t, "", "impersonation"), requireDecisions("governed"), governedDecision)
	require.NoError(t, awaitReady(t, src, 10*time.Second))
	r1 := src.Current()

	for name, publish := range map[string]func(){
		"compile failure": func() {
			server.publish(t, "impersonation", "bad", map[string]string{"/p.rego": "package company.impersonation\n\nresult := undefined_fn(1)\n"}, nil)
		},
		"prohibited builtin": func() {
			server.publish(t, "impersonation", "send", map[string]string{"/p.rego": "package company.impersonation\n\nimport rego.v1\n\nresult := http.send({\"method\": \"GET\", \"url\": \"http://127.0.0.1:1\"})\n"}, nil)
		},
		"decision absent": func() {
			server.publish(t, "impersonation", "absent", map[string]string{"/p.rego": "package other\n\nimport rego.v1\n\nresult := {\"action\": \"allow\"}\n"}, nil)
		},
		"fetch failure": func() { server.set(func(s *bundleServer) { s.failing = true }) },
	} {
		t.Run(name, func(t *testing.T) {
			before := server.requests
			publish()
			require.Eventually(t, func() bool {
				server.mu.Lock()
				defer server.mu.Unlock()
				return server.requests > before+1
			}, 10*time.Second, 50*time.Millisecond)
			assert.Same(t, r1, src.Current())
			assert.Equal(t, ports.ImpersonationPolicyAllow, evaluate(t, src.Current()))
			server.set(func(s *bundleServer) { s.failing = false })
		})
	}
}

func TestManagedSourceRequiresEveryConfiguredBundleInOneSnapshot(t *testing.T) {
	server := newBundleServer(t)
	server.publish(t, "governance", "g1", map[string]string{"/p.rego": allowPolicy}, governance(true), "company", "governance")
	server.publish(t, "lookup", "l1", map[string]string{"/lookup.rego": "package lookup\n\nimport rego.v1\n\nx := http.send({\"method\": \"GET\", \"url\": \"http://127.0.0.1:1\"})\n"}, nil, "lookup")
	src := startManaged(t, server.bootstrap(t, "", "governance", "lookup"), requireDecisions("governed"), governedDecision)

	err := awaitReady(t, src, 3*time.Second)
	var notReady *ports.ImpersonationPolicyNotReadyError
	require.ErrorAs(t, err, &notReady)
	assert.Equal(t, string(reasonProhibitedBuiltin), notReady.Reason)
	assert.Nil(t, src.Current(), "a usable first-bundle subset is never admitted")

	server.publish(t, "lookup", "l2", map[string]string{"/lookup.rego": "package lookup\n\nimport rego.v1\n\nx := 1\n"}, nil, "lookup")
	require.NoError(t, awaitReady(t, src, 10*time.Second))
	assert.Equal(t, "governance=g1,lookup=l2", src.Current().Label())
}

func TestManagedSourceAllOrNothingAdmission(t *testing.T) {
	server := newBundleServer(t)
	server.publish(t, "impersonation", "r1", map[string]string{"/p.rego": allowPolicy}, governance(true))
	second := decision("second", "company.second", "result")
	src := startManaged(t, server.bootstrap(t, "", "impersonation"), requireDecisions("governed", "second"), governedDecision, second)

	err := awaitReady(t, src, 3*time.Second)
	var notReady *ports.ImpersonationPolicyNotReadyError
	require.ErrorAs(t, err, &notReady)
	assert.Equal(t, string(reasonDecisionAbsent), notReady.Reason)
	assert.Equal(t, "second", notReady.Rule)
}

func TestManagedSourceStartupBoundWithoutBundles(t *testing.T) {
	server := newBundleServer(t)
	server.set(func(s *bundleServer) { s.failing = true })
	src := startManaged(t, server.bootstrap(t, "", "impersonation"), admitAll, governedDecision)
	start := time.Now()
	err := awaitReady(t, src, time.Second)
	require.Error(t, err)
	assert.Less(t, time.Since(start), 3*time.Second)
	assert.Nil(t, src.Current())
}

func TestManagedSourceEvaluatesWhileAFetchStalls(t *testing.T) {
	server := newBundleServer(t)
	server.publish(t, "impersonation", "r1", map[string]string{"/p.rego": allowPolicy}, governance(true))
	src := startManaged(t, server.bootstrap(t, "", "impersonation"), admitAll, governedDecision)
	require.NoError(t, awaitReady(t, src, 10*time.Second))
	server.set(func(s *bundleServer) { s.latency = 30 * time.Second })
	time.Sleep(1500 * time.Millisecond)
	start := time.Now()
	assert.Equal(t, ports.ImpersonationPolicyAllow, evaluate(t, src.Current()))
	assert.Less(t, time.Since(start), time.Second)
}

func TestManagedSourceRevisionSwapsUnderConcurrentEvaluation(t *testing.T) {
	server := newBundleServer(t)
	server.publish(t, "impersonation", "r0", map[string]string{"/p.rego": allowPolicy}, governance(true))
	src := startManaged(t, server.bootstrap(t, "", "impersonation"), admitAll, governedDecision)
	require.NoError(t, awaitReady(t, src, 10*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				rev := src.Current()
				first, err := rev.Evaluate(ctx, "governed", nil)
				if ctx.Err() != nil {
					return
				}
				assert.NoError(t, err)
				second, err := rev.Evaluate(ctx, "governed", nil)
				if ctx.Err() != nil {
					return
				}
				assert.NoError(t, err)
				assert.Equal(t, first, second, "one revision gives one answer")
			}
		}()
	}
	for i := 1; ctx.Err() == nil && i < 4; i++ {
		server.publish(t, "impersonation", "r", map[string]string{"/p.rego": allowPolicy}, governance(i%2 == 0))
		time.Sleep(1100 * time.Millisecond)
	}
	wg.Wait()
}

func TestManagedSourceStopIsIdempotent(t *testing.T) {
	server := newBundleServer(t)
	path := server.bootstrap(t, "", "impersonation")
	src, err := NewImpersonationPolicySource(ports.ImpersonationOPAEngineConfig{Policy: ports.ImpersonationOPAPolicySource{ConfigFile: path}},
		[]ports.ImpersonationPolicyDecisionRef{governedDecision}, admitAll, discardLogger())
	require.NoError(t, err)
	src.Stop(context.Background())
	src.Stop(context.Background())
}

func TestManagedSourceRejectedBundleBlocksOtherBundlesUntilRepublished(t *testing.T) {
	server := newBundleServer(t)
	lookup := func(body string) map[string]string {
		return map[string]string{"/lookup.rego": "package lookup\n\nimport rego.v1\n\n" + body + "\n"}
	}
	server.publish(t, "governance", "g1", map[string]string{"/p.rego": allowPolicy}, governance(true), "company", "governance")
	server.publish(t, "lookup", "l1", lookup("x := 1"), nil, "lookup")
	src := startManaged(t, server.bootstrap(t, "", "governance", "lookup"), admitAll, governedDecision)
	require.NoError(t, awaitReady(t, src, 10*time.Second))
	active := src.Current()

	server.publish(t, "lookup", "l2", lookup(`x := http.send({"method": "GET", "url": "http://127.0.0.1:1"})`), nil, "lookup")
	time.Sleep(1500 * time.Millisecond)
	server.publish(t, "governance", "g2", map[string]string{"/p.rego": allowPolicy}, governance(false), "company", "governance")
	time.Sleep(2500 * time.Millisecond)
	assert.Same(t, active, src.Current(), "every candidate still contains the rejected lookup bundle")

	server.publish(t, "lookup", "l3", lookup("x := 2"), nil, "lookup")
	require.Eventually(t, func() bool { return src.Current().Label() == "governance=g2,lookup=l3" }, 10*time.Second, 50*time.Millisecond)
	assert.Equal(t, ports.ImpersonationPolicyDeny, evaluate(t, src.Current()))
}
