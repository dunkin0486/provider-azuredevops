// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package azuredevops

import (
	"errors"
	"net/http"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
)

// StatusCode extracts the HTTP status code from an error returned by the
// Azure DevOps SDK, if any. It returns 0 if err is nil or doesn't carry a
// status code (e.g. a network error).
func StatusCode(err error) int {
	if err == nil {
		return 0
	}

	var wrapped azuredevops.WrappedError
	if errors.As(err, &wrapped) && wrapped.StatusCode != nil {
		return *wrapped.StatusCode
	}

	var wrappedPtr *azuredevops.WrappedError
	if errors.As(err, &wrappedPtr) && wrappedPtr.StatusCode != nil {
		return *wrappedPtr.StatusCode
	}

	return 0
}

// IsNotFound returns true if err indicates that an Azure DevOps API resource
// does not exist (HTTP 404). Resource controllers should use this in Observe
// to translate a not-found response into ResourceExists: false, and in
// Delete to treat a not-found response as a successful deletion
// (idempotent), typically via crossplane-runtime's resource.Ignore.
func IsNotFound(err error) bool {
	return StatusCode(err) == http.StatusNotFound
}

// IsThrottled returns true if err indicates that a request was rate-limited
// by the Azure DevOps API (HTTP 429). Callers may use this to decide whether
// a request is safe to retry, e.g. via Retry.
func IsThrottled(err error) bool {
	return StatusCode(err) == http.StatusTooManyRequests
}

// IsUnauthorized returns true if err indicates that the request failed
// authentication or authorization (HTTP 401 or 403), for example because the
// configured personal access token is invalid, expired, or lacks the scopes
// required for the operation.
func IsUnauthorized(err error) bool {
	code := StatusCode(err)
	return code == http.StatusUnauthorized || code == http.StatusForbidden
}

// IsRetryable returns true if err indicates a transient failure that is
// reasonable to retry: throttling (429) or a server-side error (5xx). It
// returns false for client errors (4xx, other than 429) since those are not
// expected to succeed on retry without a change in the request.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	code := StatusCode(err)
	if code == 0 {
		// No status code means the error didn't come from a decoded HTTP
		// response (e.g. a network/connection error) -- treat as
		// retryable, since these are typically transient.
		return true
	}

	return code == http.StatusTooManyRequests || code >= http.StatusInternalServerError
}
