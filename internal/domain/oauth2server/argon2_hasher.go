package oauth2server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/ory/fosite"
	"golang.org/x/crypto/argon2"
)

// Argon2id parameters per research.md Decision 4
const (
	argon2MemoryKiB   = 64 * 1024 // 64 MiB in KiB
	argon2Iterations  = 3
	argon2Parallelism = 4
	argon2SaltLength  = 16
	argon2KeyLength   = 32
)

// Argon2Hasher provides password hashing using Argon2id.
// Each hash allocates its memory parameter (64 MiB), so slots bounds how many run at once.
type Argon2Hasher struct {
	slots chan struct{}
}

// NewArgon2Hasher returns a hasher that runs at most maxConcurrent hashes at once.
func NewArgon2Hasher(maxConcurrent int) *Argon2Hasher {
	return &Argon2Hasher{slots: make(chan struct{}, maxConcurrent)}
}

// Hash generates an Argon2id hash of the given secret.
// Returns the hash in PHC string format: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
func (h *Argon2Hasher) Hash(secret string) (string, error) {
	salt := make([]byte, argon2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	hash, err := h.idKey(context.Background(), []byte(secret), salt, argon2Params{memory: argon2MemoryKiB, iterations: argon2Iterations, parallelism: argon2Parallelism})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argon2MemoryKiB,
		argon2Iterations,
		argon2Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// Compare verifies a secret against an Argon2id PHC string.
// Returns nil if the secret matches, error otherwise.
func (h *Argon2Hasher) Compare(ctx context.Context, phcString, secret string) error {
	salt, hash, params, err := decodePHC(phcString)
	if err != nil {
		return fmt.Errorf("invalid PHC string: %w", err)
	}

	computed, err := h.idKey(ctx, []byte(secret), salt, params)
	if err != nil {
		return err
	}

	if subtle.ConstantTimeCompare(hash, computed) != 1 {
		return fosite.ErrInvalidClient
	}
	return nil
}

// idKey waits for a free slot, or until ctx ends, before hashing.
func (h *Argon2Hasher) idKey(ctx context.Context, secret, salt []byte, params argon2Params) ([]byte, error) {
	select {
	case h.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-h.slots }()

	return argon2.IDKey(secret, salt, params.iterations, params.memory, params.parallelism, argon2KeyLength), nil
}

type argon2Params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

func decodePHC(phc string) (salt, hash []byte, params argon2Params, err error) {
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, nil, argon2Params{}, errors.New("not a valid argon2id PHC string")
	}

	// Parse parameters
	var m, t uint32
	var p uint8
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p)
	if err != nil {
		return nil, nil, argon2Params{}, fmt.Errorf("failed to parse parameters: %w", err)
	}

	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, argon2Params{}, fmt.Errorf("failed to decode salt: %w", err)
	}

	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, argon2Params{}, fmt.Errorf("failed to decode hash: %w", err)
	}
	if len(hash) != argon2KeyLength {
		return nil, nil, argon2Params{}, fmt.Errorf("hash must be %d bytes", argon2KeyLength)
	}

	return salt, hash, argon2Params{memory: m, iterations: t, parallelism: p}, nil
}
