// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package azuredevops

import (
	"context"
	"errors"
	"testing"
)

func TestListAll(t *testing.T) {
	pages := [][]int{{1, 2}, {3, 4}, {5}}

	fn := func(_ context.Context, token string) ([]int, string, error) {
		idx := 0
		if token != "" {
			idx = int(token[0] - 'a' + 1)
		}
		items := pages[idx]
		next := ""
		if idx+1 < len(pages) {
			next = string(rune('a' + idx))
		}
		return items, next, nil
	}

	got, err := ListAll(context.Background(), fn)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}

	want := []int{1, 2, 3, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("ListAll() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ListAll()[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestListAllError(t *testing.T) {
	wantErr := errors.New("boom")
	fn := func(_ context.Context, _ string) ([]int, string, error) {
		return nil, "", wantErr
	}

	_, err := ListAll(context.Background(), fn)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ListAll() error = %v, want %v", err, wantErr)
	}
}

func TestListAllSinglePage(t *testing.T) {
	fn := func(_ context.Context, _ string) ([]string, string, error) {
		return []string{"a", "b"}, "", nil
	}

	got, err := ListAll(context.Background(), fn)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListAll() = %v, want 2 items", got)
	}
}
