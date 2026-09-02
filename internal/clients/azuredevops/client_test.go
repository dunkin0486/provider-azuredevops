// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package azuredevops

import "testing"

func TestNewConnection(t *testing.T) {
	conn := NewConnection("https://dev.azure.com/example", "fake-pat")
	if conn == nil {
		t.Fatal("NewConnection() returned nil")
	}
	if conn.BaseUrl != "https://dev.azure.com/example" {
		t.Errorf("conn.BaseUrl = %q, want %q", conn.BaseUrl, "https://dev.azure.com/example")
	}
	if conn.AuthorizationString == "" {
		t.Error("conn.AuthorizationString is empty, want a PAT-derived auth header")
	}
}
