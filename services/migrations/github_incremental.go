// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package migrations

// The watermark-driven incremental layer for GitHub, used by the mirror
// metadata sync (SyncRepository). A sync resumes from the max updated time
// already stored per stream, so these entry points fetch only entities updated
// at or after that watermark:
//
//   - issues: server-side updated-since filter, walked updated-ascending
//     (GraphQL filterBy / REST since param).
//   - pull requests: no server-side since filter exists, so the sweep walks
//     newest-updated-first and stops at the watermark.
//   - comments/reviews: served per entity from the sweep caches on the GraphQL
//     path (see GetNewComments/GetNewReviews in github.go), fetched
//     since-filtered over REST otherwise.

import (
	"context"
	"fmt"
	"time"

	base "gitea.dev/modules/migration"

	"github.com/google/go-github/v89/github"
)

// SupportSyncing returns true: an already-migrated GitHub repository can be
// incrementally re-synced.
func (g *GithubDownloaderV3) SupportSyncing() bool {
	return true
}

// GetNewIssues returns issues updated at or after the given time, paginated.
func (g *GithubDownloaderV3) GetNewIssues(ctx context.Context, page, perPage int, updatedAfter time.Time) ([]*base.Issue, bool, error) {
	g.gqlSince = updatedAfter
	if g.useGraphQL {
		return g.getIssuesGraphQL(ctx, page, perPage)
	}
	// A resumable sync walks by UPDATE order so the max updated_unix already
	// stored is an exact resume point. (Walking by creation order would let
	// the updated-based watermark skip older-but-recently-touched issues.)
	return g.getIssuesRESTSince(ctx, page, perPage, "updated", updatedAfter)
}

// GetNewPullRequests returns pull requests updated at or after the given time,
// paginated. The pull-request list has no since filter, so both paths list by
// most recently updated and stop as soon as a pull request older than the
// watermark appears. The search API is deliberately avoided: its results are
// capped and it has a separate, much smaller rate limit.
func (g *GithubDownloaderV3) GetNewPullRequests(ctx context.Context, page, perPage int, updatedAfter time.Time) ([]*base.PullRequest, bool, error) {
	g.gqlSince = updatedAfter
	if g.useGraphQL {
		return g.getPullRequestsGraphQL(ctx, page, perPage)
	}
	return g.getPullRequestsRESTSince(ctx, page, perPage, updatedAfter)
}

// getPullRequestsRESTSince lists pull requests newest-updated-first over REST
// and stops at the watermark.
func (g *GithubDownloaderV3) getPullRequestsRESTSince(ctx context.Context, page, perPage int, updatedAfter time.Time) ([]*base.PullRequest, bool, error) {
	if perPage > g.maxPerPage {
		perPage = g.maxPerPage
	}
	opt := &github.PullRequestListOptions{
		Sort:      "updated",
		Direction: "desc",
		State:     "all",
		ListOptions: github.ListOptions{
			PerPage: perPage,
			Page:    page,
		},
	}
	g.waitAndPickClient(ctx)
	prs, resp, err := g.getClient().PullRequests.List(ctx, g.repoOwner, g.repoName, opt)
	if err != nil {
		return nil, false, fmt.Errorf("error while listing repos: %w", err)
	}
	g.setRate(&resp.Rate)
	allPRs := make([]*base.PullRequest, 0, len(prs))
	hitWatermark := false
	for _, pr := range prs {
		if pr.GetUpdatedAt().Time.Before(updatedAfter) {
			hitWatermark = true
			break
		}
		basePR, err := g.convertGithubPullRequest(ctx, pr, perPage)
		if err != nil {
			return nil, false, err
		}
		allPRs = append(allPRs, basePR)
		// SECURITY: ensure the PR is safe (mirrors the full-migration path)
		_ = CheckAndEnsureSafePR(basePR, g.baseURL, g)
	}
	return allPRs, hitWatermark || resp.NextPage == 0, nil
}
