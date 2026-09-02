// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package azuredevops

import (
	"errors"
	"net/http"
	"testing"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
)

func withStatus(code int) error {
	c := code
	return &azuredevops.WrappedError{StatusCode: &c}
}

func withStatusValue(code int) error {
	c := code
	return azuredevops.WrappedError{StatusCode: &c}
}

func TestStatusCode(t *testing.T) {
	cases := map[string]struct {
		err  error
		want int
	}{
		"Nil":              {err: nil, want: 0},
		"NoStatusCode":     {err: errors.New("boom"), want: 0},
		"PointerWrapped":   {err: withStatus(http.StatusNotFound), want: http.StatusNotFound},
		"ValueWrapped":     {err: withStatusValue(http.StatusTooManyRequests), want: http.StatusTooManyRequests},
		"WrappedFmtErrorf": {err: fmtErrorf(withStatus(http.StatusForbidden)), want: http.StatusForbidden},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := StatusCode(tc.err); got != tc.want {
				t.Errorf("StatusCode() = %d, want %d", got, tc.want)
			}
		})
	}
}

func fmtErrorf(err error) error {
	return errors.Join(err)
}

func TestIsNotFound(t *testing.T) {
	if !IsNotFound(withStatus(http.StatusNotFound)) {
		t.Error("IsNotFound() = false, want true for 404")
	}
	if IsNotFound(withStatus(http.StatusOK)) {
		t.Error("IsNotFound() = true, want false for 200")
	}
	if IsNotFound(nil) {
		t.Error("IsNotFound(nil) = true, want false")
	}
}

func TestIsThrottled(t *testing.T) {
	if !IsThrottled(withStatus(http.StatusTooManyRequests)) {
		t.Error("IsThrottled() = false, want true for 429")
	}
	if IsThrottled(withStatus(http.StatusNotFound)) {
		t.Error("IsThrottled() = true, want false for 404")
	}
}

func TestIsUnauthorized(t *testing.T) {
	if !IsUnauthorized(withStatus(http.StatusUnauthorized)) {
		t.Error("IsUnauthorized() = false, want true for 401")
	}
	if !IsUnauthorized(withStatus(http.StatusForbidden)) {
		t.Error("IsUnauthorized() = false, want true for 403")
	}
	if IsUnauthorized(withStatus(http.StatusNotFound)) {
		t.Error("IsUnauthorized() = true, want false for 404")
	}
}

func TestIsRetryable(t *testing.T) {
	cases := map[string]struct {
		err  error
		want bool
	}{
		"Nil":          {err: nil, want: false},
		"NetworkError": {err: errors.New("connection reset"), want: true},
		"Throttled":    {err: withStatus(http.StatusTooManyRequests), want: true},
		"ServerError":  {err: withStatus(http.StatusBadGateway), want: true},
		"NotFound":     {err: withStatus(http.StatusNotFound), want: false},
		"Unauthorized": {err: withStatus(http.StatusUnauthorized), want: false},
		"BadRequest":   {err: withStatus(http.StatusBadRequest), want: false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := IsRetryable(tc.err); got != tc.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tc.want)
			}
		})
	}
}
