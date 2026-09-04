package storage

import (
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

func TestToolApproval_IsExpired(t *testing.T) {
	now := time.Now()

	t.Run("not expired when before expiry", func(t *testing.T) {
		a := &ToolApproval{ExpiresAt: now.Add(10 * time.Minute)}
		if a.IsExpired(now) {
			t.Fatal("expected not expired")
		}
	})

	t.Run("expired when after expiry", func(t *testing.T) {
		a := &ToolApproval{ExpiresAt: now.Add(-1 * time.Minute)}
		if !a.IsExpired(now) {
			t.Fatal("expected expired")
		}
	})

	t.Run("expired when exactly at expiry", func(t *testing.T) {
		a := &ToolApproval{ExpiresAt: now}
		// time.After is strictly after, so equal is not expired
		if a.IsExpired(now) {
			t.Fatal("expected not expired at exact boundary")
		}
	})
}

func TestToolApproval_IsActionable(t *testing.T) {
	now := time.Now()

	t.Run("actionable when pending and not expired", func(t *testing.T) {
		a := &ToolApproval{
			Status:    ApprovalStatusPending,
			ExpiresAt: now.Add(10 * time.Minute),
		}
		if !a.IsActionable(now) {
			t.Fatal("expected actionable")
		}
	})

	t.Run("not actionable when approved", func(t *testing.T) {
		a := &ToolApproval{
			Status:    ApprovalStatusApproved,
			ExpiresAt: now.Add(10 * time.Minute),
		}
		if a.IsActionable(now) {
			t.Fatal("expected not actionable when approved")
		}
	})

	t.Run("not actionable when denied", func(t *testing.T) {
		a := &ToolApproval{
			Status:    ApprovalStatusDenied,
			ExpiresAt: now.Add(10 * time.Minute),
		}
		if a.IsActionable(now) {
			t.Fatal("expected not actionable when denied")
		}
	})

	t.Run("not actionable when expired", func(t *testing.T) {
		a := &ToolApproval{
			Status:    ApprovalStatusPending,
			ExpiresAt: now.Add(-1 * time.Minute),
		}
		if a.IsActionable(now) {
			t.Fatal("expected not actionable when expired")
		}
	})
}

func TestToolApproval_Approve(t *testing.T) {
	principal := id.Principal("user@example.com")
	now := time.Now()

	t.Run("approves pending approval with persistence", func(t *testing.T) {
		a := &ToolApproval{
			Principal: principal,
			Status:    ApprovalStatusPending,
			ExpiresAt: now.Add(10 * time.Minute),
		}
		err := a.Approve(principal, ApprovalDecision{Persistence: ApprovalPersistenceOnce, ToolPattern: "create_pull_request", ParamsPattern: map[string]string{"repo": "acme/app"}}, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.Status != ApprovalStatusApproved {
			t.Fatalf("expected approved, got %s", a.Status)
		}
		if a.Persistence == nil || *a.Persistence != ApprovalPersistenceOnce {
			t.Fatal("expected once persistence")
		}
		if a.ApprovedAt == nil || !a.ApprovedAt.Equal(now) {
			t.Fatal("expected approved_at to be set")
		}
	})

	t.Run("rejects approval with wrong principal", func(t *testing.T) {
		a := &ToolApproval{
			Principal: principal,
			Status:    ApprovalStatusPending,
			ExpiresAt: now.Add(10 * time.Minute),
		}
		err := a.Approve(id.Principal("other@example.com"), ApprovalDecision{Persistence: ApprovalPersistenceOnce}, now)
		if err != ErrApprovalPrincipalMismatch {
			t.Fatalf("expected ErrApprovalPrincipalMismatch, got %v", err)
		}
	})

	t.Run("rejects approval when not pending", func(t *testing.T) {
		a := &ToolApproval{
			Principal: principal,
			Status:    ApprovalStatusApproved,
			ExpiresAt: now.Add(10 * time.Minute),
		}
		err := a.Approve(principal, ApprovalDecision{Persistence: ApprovalPersistenceOnce}, now)
		if err != ErrApprovalNotPending {
			t.Fatalf("expected ErrApprovalNotPending, got %v", err)
		}
	})

	t.Run("rejects approval when expired", func(t *testing.T) {
		a := &ToolApproval{
			Principal: principal,
			Status:    ApprovalStatusPending,
			ExpiresAt: now.Add(-1 * time.Minute),
		}
		err := a.Approve(principal, ApprovalDecision{Persistence: ApprovalPersistenceOnce}, now)
		if err != ErrApprovalExpired {
			t.Fatalf("expected ErrApprovalExpired, got %v", err)
		}
	})

	t.Run("approves with permanent persistence", func(t *testing.T) {
		a := &ToolApproval{
			Principal: principal,
			Status:    ApprovalStatusPending,
			ExpiresAt: now.Add(10 * time.Minute),
		}
		err := a.Approve(principal, ApprovalDecision{Persistence: ApprovalPersistencePermanent, ToolPattern: "*", ParamsPattern: map[string]string{}}, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.Persistence == nil || *a.Persistence != ApprovalPersistencePermanent {
			t.Fatal("expected permanent persistence")
		}
	})

	t.Run("approves with session persistence", func(t *testing.T) {
		a := &ToolApproval{
			Principal: principal,
			Status:    ApprovalStatusPending,
			ExpiresAt: now.Add(10 * time.Minute),
		}
		err := a.Approve(principal, ApprovalDecision{Persistence: ApprovalPersistenceSession, ToolPattern: "create_pull_request", ParamsPattern: map[string]string{}}, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.Persistence == nil || *a.Persistence != ApprovalPersistenceSession {
			t.Fatal("expected session persistence")
		}
	})
}

func TestToolApproval_Deny(t *testing.T) {
	principal := id.Principal("user@example.com")
	now := time.Now()

	t.Run("denies pending approval", func(t *testing.T) {
		a := &ToolApproval{
			Principal: principal,
			Status:    ApprovalStatusPending,
			ExpiresAt: now.Add(10 * time.Minute),
		}
		err := a.Deny(principal, nil, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.Status != ApprovalStatusDenied {
			t.Fatalf("expected denied, got %s", a.Status)
		}
		if a.DeniedAt == nil || !a.DeniedAt.Equal(now) {
			t.Fatal("expected denied_at to be set")
		}
	})

	t.Run("denies with permanent persistence", func(t *testing.T) {
		a := &ToolApproval{
			Principal: principal,
			Status:    ApprovalStatusPending,
			ExpiresAt: now.Add(10 * time.Minute),
		}
		p := ApprovalPersistencePermanent
		err := a.Deny(principal, &p, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.Persistence == nil || *a.Persistence != ApprovalPersistencePermanent {
			t.Fatal("expected permanent persistence")
		}
	})

	t.Run("rejects deny with wrong principal", func(t *testing.T) {
		a := &ToolApproval{
			Principal: principal,
			Status:    ApprovalStatusPending,
			ExpiresAt: now.Add(10 * time.Minute),
		}
		err := a.Deny(id.Principal("other@example.com"), nil, now)
		if err != ErrApprovalPrincipalMismatch {
			t.Fatalf("expected ErrApprovalPrincipalMismatch, got %v", err)
		}
	})

	t.Run("rejects deny when not pending", func(t *testing.T) {
		a := &ToolApproval{
			Principal: principal,
			Status:    ApprovalStatusDenied,
			ExpiresAt: now.Add(10 * time.Minute),
		}
		err := a.Deny(principal, nil, now)
		if err != ErrApprovalNotPending {
			t.Fatalf("expected ErrApprovalNotPending, got %v", err)
		}
	})

	t.Run("rejects deny when expired", func(t *testing.T) {
		a := &ToolApproval{
			Principal: principal,
			Status:    ApprovalStatusPending,
			ExpiresAt: now.Add(-1 * time.Minute),
		}
		err := a.Deny(principal, nil, now)
		if err != ErrApprovalExpired {
			t.Fatalf("expected ErrApprovalExpired, got %v", err)
		}
	})
}

func TestToolApproval_Consume(t *testing.T) {
	now := time.Now()
	once := ApprovalPersistenceOnce

	t.Run("consumes once-persistence approved approval", func(t *testing.T) {
		a := &ToolApproval{
			Status:      ApprovalStatusApproved,
			Persistence: &once,
			Consumed:    false,
		}
		err := a.Consume(now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !a.Consumed {
			t.Fatal("expected consumed")
		}
		if a.ConsumedAt == nil || !a.ConsumedAt.Equal(now) {
			t.Fatal("expected consumed_at to be set")
		}
	})

	t.Run("consume is idempotent", func(t *testing.T) {
		consumedTime := now.Add(-1 * time.Minute)
		a := &ToolApproval{
			Status:      ApprovalStatusApproved,
			Persistence: &once,
			Consumed:    true,
			ConsumedAt:  &consumedTime,
		}
		err := a.Consume(now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should keep original consumed_at
		if !a.ConsumedAt.Equal(consumedTime) {
			t.Fatal("idempotent consume should not update consumed_at")
		}
	})

	t.Run("rejects consume when not approved", func(t *testing.T) {
		a := &ToolApproval{
			Status:      ApprovalStatusPending,
			Persistence: &once,
		}
		err := a.Consume(now)
		if err != ErrApprovalNotApproved {
			t.Fatalf("expected ErrApprovalNotApproved, got %v", err)
		}
	})

	t.Run("rejects consume when session persistence", func(t *testing.T) {
		session := ApprovalPersistenceSession
		a := &ToolApproval{
			Status:      ApprovalStatusApproved,
			Persistence: &session,
		}
		err := a.Consume(now)
		if err != ErrApprovalNotOnce {
			t.Fatalf("expected ErrApprovalNotOnce, got %v", err)
		}
	})

	t.Run("rejects consume when permanent persistence", func(t *testing.T) {
		perm := ApprovalPersistencePermanent
		a := &ToolApproval{
			Status:      ApprovalStatusApproved,
			Persistence: &perm,
		}
		err := a.Consume(now)
		if err != ErrApprovalNotOnce {
			t.Fatalf("expected ErrApprovalNotOnce, got %v", err)
		}
	})
}

func TestComputeArgumentsHash(t *testing.T) {
	t.Run("produces deterministic hash", func(t *testing.T) {
		args := map[string]any{"b": 2, "a": 1}
		hash1 := ComputeArgumentsHash(args)
		hash2 := ComputeArgumentsHash(args)
		if hash1 != hash2 {
			t.Fatalf("expected deterministic hash, got %s and %s", hash1, hash2)
		}
	})

	t.Run("key order does not affect hash", func(t *testing.T) {
		args1 := map[string]any{"a": 1, "b": 2, "c": 3}
		args2 := map[string]any{"c": 3, "a": 1, "b": 2}
		if ComputeArgumentsHash(args1) != ComputeArgumentsHash(args2) {
			t.Fatal("expected same hash regardless of key order")
		}
	})

	t.Run("different args produce different hashes", func(t *testing.T) {
		args1 := map[string]any{"a": 1}
		args2 := map[string]any{"a": 2}
		if ComputeArgumentsHash(args1) == ComputeArgumentsHash(args2) {
			t.Fatal("expected different hashes for different args")
		}
	})

	t.Run("nil args produce consistent hash", func(t *testing.T) {
		hash1 := ComputeArgumentsHash(nil)
		hash2 := ComputeArgumentsHash(nil)
		if hash1 != hash2 {
			t.Fatal("expected consistent hash for nil args")
		}
	})
}

func TestToolApprovalPatterns(t *testing.T) {
	t.Run("approve records the complete decision", func(t *testing.T) {
		now := time.Now()
		approval := &ToolApproval{Principal: id.Principal("user@example.com"), Status: ApprovalStatusPending, ExpiresAt: now.Add(time.Minute)}
		decision := ApprovalDecision{Persistence: ApprovalPersistencePermanent, ToolPattern: "issues.*", ParamsPattern: map[string]string{"repo": "acme/*"}}
		if err := approval.Approve(approval.Principal, decision, now); err != nil {
			t.Fatal(err)
		}
		if approval.ToolPattern != decision.ToolPattern || approval.ParamsPattern["repo"] != "acme/*" {
			t.Fatalf("approval did not retain decision: %#v", approval)
		}
	})

	t.Run("initializes escaped exact patterns", func(t *testing.T) {
		approval := &ToolApproval{ToolName: "create_pull_request", Arguments: map[string]any{"repo": "a*b", "meta": map[string]any{"b": 1, "a": []any{"x"}}}}
		approval.ApplyExactPatterns()
		if approval.ToolPattern != "create_pull_request" || approval.ParamsPattern["repo"] != "a\\*b" || approval.ParamsPattern["meta"] != `{"a":["x"],"b":1}` {
			t.Fatalf("unexpected exact patterns: %#v", approval)
		}
	})

	t.Run("initializes non-nil empty patterns for nil arguments", func(t *testing.T) {
		approval := &ToolApproval{ToolName: "create_pull_request"}
		approval.ApplyExactPatterns()
		if approval.ParamsPattern == nil || len(approval.ParamsPattern) != 0 {
			t.Fatalf("expected non-nil empty params pattern, got %#v", approval.ParamsPattern)
		}
	})
}
