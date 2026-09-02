// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package azuredevops

import "context"

// A PageFunc fetches a single page of items from an Azure DevOps list API,
// given a continuation token (empty for the first page, per the SDK's
// X-MS-ContinuationToken convention -- see e.g. core.Client.GetProjects's
// GetProjectsArgs.ContinuationToken). It returns the items on that page and
// the continuation token for the next page, or an empty string if there are
// no more pages.
type PageFunc[T any] func(ctx context.Context, continuationToken string) (items []T, nextToken string, err error)

// ListAll drains every page returned by fn, starting with an empty
// continuation token, and returns the concatenated results. Resource
// controllers should use this in Observe/list implementations instead of
// hand-rolling continuation-token loops, so pagination behavior stays
// consistent across resources.
func ListAll[T any](ctx context.Context, fn PageFunc[T]) ([]T, error) {
	var all []T

	token := ""
	for {
		items, next, err := fn(ctx, token)
		if err != nil {
			return nil, err
		}

		all = append(all, items...)

		if next == "" {
			return all, nil
		}
		token = next
	}
}
