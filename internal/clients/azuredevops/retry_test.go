// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package azuredevops

import (
	"context"
	"net/http"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

func fastBackoff() wait.Backoff {
	return wait.Backoff{Duration: time.Millisecond, Factor: 1.0, Steps: 4}
}

func TestRetrySucceedsAfterTransientErrors(t *testing.T) {
	attempts := 0
	err := Retry(context.Background(), fastBackoff(), func() error {
		attempts++
		if attempts < 3 {
			return withStatus(http.StatusTooManyRequests)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Retry() error = %v, want nil", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestRetryStopsOnNonRetryableError(t *testing.T) {
	attempts := 0
	wantErr := withStatus(http.StatusNotFound)
	err := Retry(context.Background(), fastBackoff(), func() error {
		attempts++
		return wantErr
	})

	if err != wantErr { //nolint:errorlint // want the exact sentinel returned.
		t.Fatalf("Retry() error = %v, want %v", err, wantErr)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (should not retry non-retryable errors)", attempts)
	}
}

func TestRetryExhaustsAttempts(t *testing.T) {
	attempts := 0
	err := Retry(context.Background(), fastBackoff(), func() error {
		attempts++
		return withStatus(http.StatusTooManyRequests)
	})

	if err == nil {
		t.Fatal("Retry() error = nil, want non-nil after exhausting attempts")
	}
	if attempts != 4 {
		t.Errorf("attempts = %d, want 4 (Steps)", attempts)
	}
}
