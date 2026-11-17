package bootstrap

import (
	"context"
	"sync"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/enduser"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
)

type EvaluationSecurityObservation struct {
	Holder                    *security.CaptureHolder
	FinalizedBeforeEvaluation bool
}

type EvaluationSecurityObserver struct {
	next         enduser.AccessEvaluator
	mu           sync.Mutex
	observations []EvaluationSecurityObservation
}

// ObserveAuthZENEvaluation decorates the production service without replacing validation or resolution.
func ObserveAuthZENEvaluation(application *app.App) *EvaluationSecurityObserver {
	handler := application.EnduserHandlers.AuthZENEvaluation
	if handler == nil {
		return nil
	}
	observer := &EvaluationSecurityObserver{next: handler.Evaluator}
	handler.Evaluator = observer
	return observer
}

func (o *EvaluationSecurityObserver) Evaluate(ctx context.Context, req tokenexchange.AccessEvaluationRequest, assertion string) tokenexchange.DelegationDecision {
	holder, _ := security.CaptureHolderFromContext(ctx)
	_, finalized := security.FromContext(ctx)
	o.mu.Lock()
	o.observations = append(o.observations, EvaluationSecurityObservation{Holder: holder, FinalizedBeforeEvaluation: finalized})
	o.mu.Unlock()
	return o.next.Evaluate(ctx, req, assertion)
}

func (o *EvaluationSecurityObserver) Snapshot() []EvaluationSecurityObservation {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]EvaluationSecurityObservation(nil), o.observations...)
}
