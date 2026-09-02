// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package azuredevops

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

// DefaultBackoff is the retry/backoff schedule used by Retry when none is
// supplied. It performs up to 4 attempts total (the initial attempt plus 3
// retries), with an exponential delay starting at 500ms.
var DefaultBackoff = wait.Backoff{
	Duration: 500 * time.Millisecond,
	Factor:   2.0,
	Steps:    4,
}

// Retry calls fn, retrying with the given backoff schedule while the error
// it returns is retryable (per IsRetryable), e.g. throttling (429) or a
// transient server-side/network failure. It returns the first non-retryable
// error, or the last error if all attempts are exhausted. Resource
// controllers should wrap individual Azure DevOps API calls with Retry
// rather than implementing their own retry loops, so backoff behavior stays
// consistent across resources.
func Retry(ctx context.Context, backoff wait.Backoff, fn func() error) error {
	var lastErr error

	_ = wait.ExponentialBackoffWithContext(ctx, backoff, func(_ context.Context) (bool, error) {
		lastErr = fn()
		if lastErr == nil {
			return true, nil
		}
		if !IsRetryable(lastErr) {
			// Stop retrying, but surface the error via lastErr rather than
			// the wait.Backoff error, which loses the underlying cause.
			return true, nil
		}
		return false, nil
	})

	return lastErr
}
