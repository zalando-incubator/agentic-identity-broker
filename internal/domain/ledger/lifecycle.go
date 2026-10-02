package ledger

import (
	"context"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func ValidateErasureSubject(subject id.Principal) error {
	if strings.TrimSpace(subject.String()) == "" || strings.ContainsRune(subject.String(), '\x00') {
		return storage.NewStorageError("BusinessEvents.EraseSubject", storage.ErrorKindValidation, nil, "erasure requires an exact nonempty subject")
	}
	return nil
}

func NormalizeRetention(retention time.Duration) (time.Duration, error) {
	const maximum = (time.Duration(1<<63-1) / time.Microsecond) * time.Microsecond
	if retention <= 0 || retention > maximum {
		return 0, storage.NewStorageError("BusinessEvents.SetRetentionPolicy", storage.ErrorKindValidation, nil, "invalid retention policy")
	}
	if remainder := retention % time.Microsecond; remainder != 0 {
		retention += time.Microsecond - remainder
	}
	return retention, nil
}

func (s *Service) EraseSubject(ctx context.Context, subject id.Principal) (int64, error) {
	if err := ValidateErasureSubject(subject); err != nil {
		return 0, err
	}
	return s.lifecycle.EraseSubject(ctx, subject)
}

func (s *Service) ApplyRetention(ctx context.Context) error {
	return s.lifecycle.ApplyRetention(ctx)
}

func (s *Service) SetRetentionPolicy(ctx context.Context, retention time.Duration) error {
	normalized, err := NormalizeRetention(retention)
	if err != nil {
		return err
	}
	return s.lifecycle.SetRetentionPolicy(ctx, normalized)
}
