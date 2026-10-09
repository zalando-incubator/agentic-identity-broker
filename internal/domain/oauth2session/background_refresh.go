package oauth2session

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type refreshFlightResult struct {
	session   *storage.UserSession
	refreshed bool
	trigger   RefreshTrigger
	client    refreshClientInfo
}

type backgroundRefresher struct {
	service  *OAuth2SessionService
	slots    chan struct{}
	inflight sync.Map
	baseCtx  context.Context
	cancel   context.CancelFunc
	tracer   trace.Tracer

	mu      sync.Mutex
	active  int
	closed  bool
	drained chan struct{}

	triggered metric.Int64Counter
	dropped   metric.Int64Counter
	failed    metric.Int64Counter
	label     metric.AddOption
}

func newBackgroundRefresher(service *OAuth2SessionService, workers int) *backgroundRefresher {
	meter := otel.Meter("oauth2session")
	triggered, _ := meter.Int64Counter("proactive_refresh_triggered_total",
		metric.WithDescription("Background session refreshes admitted to the pool"), metric.WithUnit("{refresh}"))
	dropped, _ := meter.Int64Counter("proactive_refresh_dropped_total",
		metric.WithDescription("Background session refreshes dropped because the pool is full"), metric.WithUnit("{refresh}"))
	failed, _ := meter.Int64Counter("proactive_refresh_failed_total",
		metric.WithDescription("Admitted background session refreshes that failed"), metric.WithUnit("{refresh}"))
	baseCtx, cancel := context.WithCancel(context.Background())
	return &backgroundRefresher{
		service: service, slots: make(chan struct{}, workers),
		baseCtx: baseCtx, cancel: cancel, tracer: otel.Tracer("oauth2session"),
		drained:   make(chan struct{}),
		triggered: triggered, dropped: dropped, failed: failed,
		label: metric.WithAttributes(attribute.String("triggered_by", string(RefreshTriggerBackground))),
	}
}

func (p *backgroundRefresher) submit(ctx context.Context, session *storage.UserSession, lookahead time.Duration) {
	principal, serviceID, sessionID := session.Principal, session.ServiceID, session.ID
	key := principal.String() + "|" + serviceID.String()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	if _, present := p.inflight.LoadOrStore(key, struct{}{}); present {
		p.mu.Unlock()
		return
	}
	select {
	case p.slots <- struct{}{}:
		p.active++
	default:
		p.inflight.Delete(key)
		p.mu.Unlock()
		p.dropped.Add(ctx, 1, p.label)
		p.service.logger.WarnContext(ctx, "oauth2 proactive refresh dropped",
			"event", "session.oauth2.proactive_refresh_dropped",
			"session_id", sessionID.String(), "service_id", serviceID.String(),
			"triggered_by", RefreshTriggerBackground, "workers", cap(p.slots))
		return
	}
	p.mu.Unlock()
	p.triggered.Add(ctx, 1, p.label)
	origin := trace.SpanContextFromContext(ctx)
	go p.refresh(key, principal, serviceID, sessionID, lookahead, origin)
}

func (p *backgroundRefresher) refresh(key string, principal id.Principal, serviceID id.ServiceID, sessionID id.SessionID, lookahead time.Duration, origin trace.SpanContext) {
	defer p.finish(key)
	options := []trace.SpanStartOption{trace.WithNewRoot()}
	if origin.IsValid() {
		options = append(options, trace.WithLinks(trace.Link{SpanContext: origin}))
	}
	spanCtx, span := p.tracer.Start(p.baseCtx, "oauth2session.background_refresh", options...)
	defer span.End()
	span.SetAttributes(attribute.String("oauth2.refresh.triggered_by", string(RefreshTriggerBackground)))
	refreshCtx, cancel := p.service.refreshOperationContext(spanCtx)
	defer cancel()

	value, err, _ := p.service.refreshGroup.Do(key, func() (any, error) {
		current, changed, provider, err := p.service.refreshDueSession(refreshCtx, principal, serviceID, time.Now().Add(lookahead), RefreshTriggerBackground)
		return refreshFlightResult{session: current, refreshed: changed, trigger: RefreshTriggerBackground, client: clientInfoFromProvider(provider)}, err
	})
	flight := value.(refreshFlightResult)
	if err == nil {
		flight, err = p.continueSharedRefresh(refreshCtx, principal, serviceID, lookahead, flight)
	}
	if err == nil {
		outcome := "not_due"
		if flight.refreshed {
			outcome = "refreshed"
		}
		span.SetAttributes(attribute.String("oauth2.refresh.outcome", outcome))
		return
	}
	metadata := sessionFailureMetadata(err)
	if errors.Is(err, ErrSessionNotFound) || metadata.Detail() == DetailSessionMissing {
		span.SetAttributes(attribute.String("oauth2.refresh.outcome", "not_due"))
		return
	}
	span.SetAttributes(attribute.String("oauth2.refresh.outcome", "failed"))
	p.failed.Add(refreshCtx, 1, p.label)
	level := slog.LevelError
	switch metadata.Kind() {
	case KindProvider, KindSession, KindConfiguration, KindCanceled:
		level = slog.LevelWarn
	case KindInfrastructure:
		if metadata.Dependency() == DependencyProvider {
			level = slog.LevelWarn
		}
	}
	attrs := []slog.Attr{
		slog.String("event", "session.oauth2.refresh_failed"),
		slog.String("session_id", sessionID.String()),
		slog.String("service_id", serviceID.String()),
		slog.String("triggered_by", string(RefreshTriggerBackground)),
		slog.Any("oauth2_session", metadata),
		slog.Int64("timestamp", time.Now().Unix()),
	}
	if flight.client.known {
		attrs = append(attrs, slog.Bool("public_client", flight.client.public))
	}
	p.service.logger.LogAttrs(refreshCtx, level, "oauth2_refresh_failed", attrs...)
}

func (p *backgroundRefresher) continueSharedRefresh(ctx context.Context, principal id.Principal, serviceID id.ServiceID, lookahead time.Duration, flight refreshFlightResult) (refreshFlightResult, error) {
	if flight.trigger == RefreshTriggerOnDemand && !flight.refreshed && flight.session != nil && flight.session.AccessTokenExpiresBy(time.Now().Add(lookahead)) {
		current, refreshed, provider, err := p.service.refreshDueSession(ctx, principal, serviceID, time.Now().Add(lookahead), RefreshTriggerBackground)
		return refreshFlightResult{session: current, refreshed: refreshed, trigger: RefreshTriggerBackground, client: clientInfoFromProvider(provider)}, err
	}
	return flight, nil
}

func (p *backgroundRefresher) finish(key string) {
	p.mu.Lock()
	p.inflight.Delete(key)
	<-p.slots
	p.active--
	if p.closed && p.active == 0 {
		close(p.drained)
	}
	p.mu.Unlock()
}

func (p *backgroundRefresher) Close(ctx context.Context) error {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		if p.active == 0 {
			close(p.drained)
		}
	}
	p.mu.Unlock()
	select {
	case <-p.drained:
		p.cancel()
		return nil
	case <-ctx.Done():
		p.cancel()
		return ctx.Err()
	}
}
