package ports

import (
	"context"
	"slices"
)

type storageTransactionHintsKey struct{}
type storageTransactionEffectsKey struct{}

func WithStorageTransactionHints(ctx context.Context, hints StorageTransactionHints) context.Context {
	hints.Subjects = slices.Clone(hints.Subjects)
	return context.WithValue(ctx, storageTransactionHintsKey{}, hints)
}

func StorageTransactionHintsFromContext(ctx context.Context) StorageTransactionHints {
	hints, _ := ctx.Value(storageTransactionHintsKey{}).(StorageTransactionHints)
	hints.Subjects = slices.Clone(hints.Subjects)
	return hints
}

func WithStorageTransactionEffects(ctx context.Context, effects StorageTransactionEffects) context.Context {
	return context.WithValue(ctx, storageTransactionEffectsKey{}, effects)
}

func StorageTransactionEffectsFromContext(ctx context.Context) (StorageTransactionEffects, bool) {
	effects, ok := ctx.Value(storageTransactionEffectsKey{}).(StorageTransactionEffects)
	return effects, ok
}
