package security

import "context"

type maintenanceAuditOriginKey struct{}

// WithMaintenanceAuditOrigin marks a non-request transition so request metadata
// cannot be attributed to an expiry or offline invalidation.
func WithMaintenanceAuditOrigin(ctx context.Context) context.Context {
	return context.WithValue(ctx, maintenanceAuditOriginKey{}, true)
}

// RedactedAuditFields contains only request identifiers resolved by the
// security middleware. A request target or raw headers may contain credentials.
type RedactedAuditFields struct {
	Origin    string
	ClientIP  string
	UserAgent string
}

// AuditContextFields returns the redacted log projection without allocating a receipt map.
func AuditContextFields(ctx context.Context) RedactedAuditFields {
	if ctx.Value(maintenanceAuditOriginKey{}) == true {
		return RedactedAuditFields{Origin: "maintenance"}
	}
	fields := RedactedAuditFields{Origin: "request"}
	if trusted, ok := FromContext(ctx); ok {
		fields.ClientIP = trusted.ClientIP
		fields.UserAgent = TruncateUserAgent(trusted.UserAgent)
	}
	return fields
}

// RedactedAuditContext is the durable receipt projection of those fields.
func RedactedAuditContext(ctx context.Context) map[string]string {
	fields := AuditContextFields(ctx)
	redacted := map[string]string{"origin": fields.Origin}
	if fields.ClientIP != "" {
		redacted["client_ip"] = fields.ClientIP
	}
	if fields.UserAgent != "" {
		redacted["user_agent"] = fields.UserAgent
	}
	return redacted
}
