// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package secrethash

import "testing"

func TestHashAndMatches(t *testing.T) {
	const secret = "s3cr3t-value"

	got, err := Hash(secret)
	if err != nil {
		t.Fatalf("Hash(...): unexpected error: %v", err)
	}
	if got == "" {
		t.Fatalf("Hash(...): got empty digest")
	}
	if !Matches(secret, got) {
		t.Fatalf("Matches(%q, %q) = false, want true", secret, got)
	}
	if Matches("wrong-value", got) {
		t.Fatalf("Matches(wrong-value, %q) = true, want false", got)
	}
}

func TestHashIsSalted(t *testing.T) {
	const secret = "s3cr3t-value"

	a, err := Hash(secret)
	if err != nil {
		t.Fatalf("Hash(...): unexpected error: %v", err)
	}
	b, err := Hash(secret)
	if err != nil {
		t.Fatalf("Hash(...): unexpected error: %v", err)
	}
	if a == b {
		t.Fatalf("Hash(...) returned identical digests for two calls with the same secret, want different (random) salts: %q", a)
	}
	if !Matches(secret, a) || !Matches(secret, b) {
		t.Fatalf("both digests should still match the same secret")
	}
}

func TestMatchesRejectsMalformed(t *testing.T) {
	cases := []string{
		"",
		"not-a-valid-encoding",
		"zz:zz",
		"deadbeef:zz",
	}
	for _, encoded := range cases {
		if Matches("anything", encoded) {
			t.Errorf("Matches(anything, %q) = true, want false", encoded)
		}
	}
}
