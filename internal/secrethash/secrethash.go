// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package secrethash provides a salted, computationally-expensive digest for
// fingerprinting secret values (passwords, tokens, client secrets) so
// controllers can detect when a referenced Kubernetes Secret's value has been
// rotated, without persisting the plaintext value itself.
//
// Azure DevOps never returns stored service endpoint/variable group secrets
// back on read, so Observe can only detect drift in secret-backed fields by
// comparing a digest of the currently-referenced Secret's value against one
// captured at the last Create/Update. That digest is stored in a CR
// annotation, which is readable by anyone with RBAC access to the CR, so a
// bare fast hash (e.g. plain SHA-256) would let an attacker with that access
// brute-force or rainbow-table the original secret value. PBKDF2 with a
// random per-call salt is used instead to make that impractical.
package secrethash

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
)

const (
	// iterations is the PBKDF2 iteration count. This digest is used for
	// drift-detection (not an interactive login check), so a moderate cost
	// that keeps reconciliation fast while still resisting offline
	// brute-force is used.
	iterations = 210000
	keyLen     = 32
	saltLen    = 16
)

// Hash returns a salted PBKDF2-HMAC-SHA256 digest of secret, encoded as
// "<hex-salt>:<hex-digest>". A fresh random salt is generated on every call
// so the stored digest can't be used as a rainbow-table lookup key, even
// across multiple resources referencing the same secret value.
func Hash(secret string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", errors.Wrap(err, "cannot generate secret hash salt")
	}
	digest, err := pbkdf2.Key(sha256.New, secret, salt, iterations, keyLen)
	if err != nil {
		return "", errors.Wrap(err, "cannot derive secret hash")
	}
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(digest), nil
}

// Matches reports whether secret matches the digest previously produced by
// Hash and stored in encoded. Returns false (no error) for a malformed,
// empty, or legacy (e.g. plain-SHA-256) encoded value -- this is treated as
// a hash mismatch, which correctly triggers a resync rather than a panic.
func Matches(secret, encoded string) bool {
	salt, want, ok := strings.Cut(encoded, ":")
	if !ok {
		return false
	}
	saltBytes, err := hex.DecodeString(salt)
	if err != nil {
		return false
	}
	wantBytes, err := hex.DecodeString(want)
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, secret, saltBytes, iterations, len(wantBytes))
	if err != nil {
		return false
	}
	return hmac.Equal(got, wantBytes)
}
