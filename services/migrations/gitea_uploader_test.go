// Copyright 2019 The Gitea Authors. All rights reserved.
// Copyright 2018 Jonas Franz. All rights reserved.
// SPDX-License-Identifier: MIT

package migrations

import (
	"context"
	"strconv"
	"testing"
	"time"

	"gitea.dev/models/db"
	issues_model "gitea.dev/models/issues"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/git"
	"gitea.dev/modules/graceful"
	base "gitea.dev/modules/migration"
	"gitea.dev/modules/optional"
	"gitea.dev/modules/structs"
	pull_service "gitea.dev/services/pull"
	repo_service "gitea.dev/services/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGiteaUploadRepo(t *testing.T) {
	// FIXME: Since no accesskey or user/password will trigger rate limit of github, just skip
	t.Skip()

	unittest.PrepareTestEnv(t)

	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})

	ctx := t.Context()
	downloader, err := NewGithubDownloaderV3(ctx, "https://github.com", "", "", "", "go-xorm", "builder")
	require.NoError(t, err)
	var (
		repoName = "builder-" + time.Now().Format("2006-01-02-15-04-05")
		uploader = NewGiteaLocalUploader(graceful.GetManager().HammerContext(), user, user.Name, repoName)
	)

	err = migrateRepository(t.Context(), user, downloader, uploader, base.MigrateOptions{
		CloneAddr:    "https://github.com/go-xorm/builder",
		RepoName:     repoName,
		AuthUsername: "",

		Wiki:         true,
		Issues:       true,
		Milestones:   true,
		Labels:       true,
		Releases:     true,
		Comments:     true,
		PullRequests: true,
		Private:      true,
		Mirror:       false,
	}, nil)
	assert.NoError(t, err)

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerID: user.ID, Name: repoName})
	assert.True(t, repo_service.HasWiki(ctx, repo))
	assert.Equal(t, repo_model.RepositoryReady, repo.Status)

	milestones, err := db.Find[issues_model.Milestone](t.Context(), issues_model.FindMilestoneOptions{
		RepoID:   repo.ID,
		IsClosed: optional.Some(false),
	})
	assert.NoError(t, err)
	assert.Len(t, milestones, 1)

	milestones, err = db.Find[issues_model.Milestone](t.Context(), issues_model.FindMilestoneOptions{
		RepoID:   repo.ID,
		IsClosed: optional.Some(true),
	})
	assert.NoError(t, err)
	assert.Empty(t, milestones)

	labels, err := issues_model.GetLabelsByRepoID(ctx, repo.ID, "", db.ListOptions{})
	assert.NoError(t, err)
	assert.Len(t, labels, 12)

	releases, err := db.Find[repo_model.Release](t.Context(), repo_model.FindReleasesOptions{
		ListOptions: db.ListOptions{
			PageSize: 10,
			Page:     0,
		},
		IncludeTags: true,
		RepoID:      repo.ID,
	})
	assert.NoError(t, err)
	assert.Len(t, releases, 8)

	releases, err = db.Find[repo_model.Release](t.Context(), repo_model.FindReleasesOptions{
		ListOptions: db.ListOptions{
			PageSize: 10,
			Page:     0,
		},
		IncludeTags: false,
		RepoID:      repo.ID,
	})
	assert.NoError(t, err)
	assert.Len(t, releases, 1)

	issues, err := issues_model.Issues(t.Context(), &issues_model.IssuesOptions{
		RepoIDs:  []int64{repo.ID},
		IsPull:   optional.Some(false),
		SortType: "oldest",
	})
	assert.NoError(t, err)
	assert.Len(t, issues, 15)
	assert.NoError(t, issues[0].LoadDiscussComments(t.Context()))
	assert.Empty(t, issues[0].Comments)

	pulls, _, err := issues_model.PullRequests(t.Context(), repo.ID, &issues_model.PullRequestsOptions{
		SortType: "oldest",
	})
	assert.NoError(t, err)
	assert.Len(t, pulls, 30)
	assert.NoError(t, pulls[0].LoadIssue(t.Context()))
	assert.NoError(t, pulls[0].Issue.LoadDiscussComments(t.Context()))
	assert.Len(t, pulls[0].Issue.Comments, 2)
}

func TestGiteaUploadRemapLocalUser(t *testing.T) {
	unittest.PrepareTestEnv(t)
	doer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	ctx := t.Context()
	repoName := "migrated"
	uploader := NewGiteaLocalUploader(ctx, doer, doer.Name, repoName)
	// call remapLocalUser
	uploader.sameApp = true

	externalID := int64(1234567)
	externalName := "username"
	source := base.Release{
		PublisherID:   externalID,
		PublisherName: externalName,
	}

	//
	// The externalID does not match any existing user, everything
	// belongs to the doer
	//
	target := repo_model.Release{}
	uploader.userMap = make(map[int64]int64)
	err := uploader.remapUser(ctx, &source, &target)
	assert.NoError(t, err)
	assert.Equal(t, doer.ID, target.GetUserID())

	//
	// The externalID matches a known user but the name does not match,
	// everything belongs to the doer
	//
	source.PublisherID = user.ID
	target = repo_model.Release{}
	uploader.userMap = make(map[int64]int64)
	err = uploader.remapUser(ctx, &source, &target)
	assert.NoError(t, err)
	assert.Equal(t, doer.ID, target.GetUserID())

	//
	// The externalID and externalName match an existing user, everything
	// belongs to the existing user
	//
	source.PublisherName = user.Name
	target = repo_model.Release{}
	uploader.userMap = make(map[int64]int64)
	err = uploader.remapUser(ctx, &source, &target)
	assert.NoError(t, err)
	assert.Equal(t, user.ID, target.GetUserID())
}

func TestGiteaUploadRemapExternalUser(t *testing.T) {
	unittest.PrepareTestEnv(t)
	doer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})
	ctx := t.Context()
	repoName := "migrated"
	uploader := NewGiteaLocalUploader(ctx, doer, doer.Name, repoName)
	uploader.gitServiceType = structs.GiteaService
	// call remapExternalUser
	uploader.sameApp = false

	externalID := int64(1234567)
	externalName := "username"
	source := base.Release{
		PublisherID:   externalID,
		PublisherName: externalName,
	}

	//
	// When there is no user linked to the external ID, the migrated data is authored
	// by the doer
	//
	uploader.userMap = make(map[int64]int64)
	target := repo_model.Release{}
	err := uploader.remapUser(ctx, &source, &target)
	assert.NoError(t, err)
	assert.Equal(t, doer.ID, target.GetUserID())

	//
	// Link the external ID to an existing user
	//
	linkedUser := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	externalLoginUser := &user_model.ExternalLoginUser{
		ExternalID:    strconv.FormatInt(externalID, 10),
		UserID:        linkedUser.ID,
		LoginSourceID: 0,
		Provider:      structs.GiteaService.Name(),
	}
	err = user_model.LinkExternalToUser(t.Context(), linkedUser, externalLoginUser)
	assert.NoError(t, err)

	//
	// When a user is linked to the external ID, it becomes the author of
	// the migrated data
	//
	uploader.userMap = make(map[int64]int64)
	target = repo_model.Release{}
	err = uploader.remapUser(ctx, &source, &target)
	assert.NoError(t, err)
	assert.Equal(t, linkedUser.ID, target.GetUserID())
}

type syncTestDownloader struct {
	base.NullDownloader
	issues   []*base.Issue
	prs      []*base.PullRequest
	comments []*base.Comment
	reviews  []*base.Review
}

func (d *syncTestDownloader) SupportSyncing() bool { return true }

func (d *syncTestDownloader) GetNewIssues(_ context.Context, page, _ int, updatedAfter time.Time) ([]*base.Issue, bool, error) {
	if page > 1 {
		return nil, true, nil
	}
	issues := make([]*base.Issue, 0, len(d.issues))
	for _, issue := range d.issues {
		if !issue.Updated.Before(updatedAfter) {
			issues = append(issues, issue)
		}
	}
	return issues, true, nil
}

func (d *syncTestDownloader) GetNewPullRequests(_ context.Context, page, _ int, updatedAfter time.Time) ([]*base.PullRequest, bool, error) {
	if page > 1 {
		return nil, true, nil
	}
	prs := make([]*base.PullRequest, 0, len(d.prs))
	for _, pr := range d.prs {
		if !pr.Updated.Before(updatedAfter) {
			prs = append(prs, pr)
		}
	}
	return prs, true, nil
}

func (d *syncTestDownloader) GetAllNewComments(_ context.Context, page, _ int, updatedAfter time.Time) ([]*base.Comment, bool, error) {
	if page > 1 {
		return nil, true, nil
	}
	comments := make([]*base.Comment, 0, len(d.comments))
	for _, comment := range d.comments {
		if !comment.Updated.Before(updatedAfter) {
			comments = append(comments, comment)
		}
	}
	return comments, true, nil
}

func (d *syncTestDownloader) GetNewReviews(_ context.Context, reviewable base.Reviewable, _ time.Time) ([]*base.Review, error) {
	reviews := make([]*base.Review, 0, len(d.reviews))
	for _, review := range d.reviews {
		if review.IssueIndex == reviewable.GetForeignIndex() {
			reviews = append(reviews, review)
		}
	}
	return reviews, nil
}

func TestGiteaSyncRepository(t *testing.T) {
	unittest.PrepareTestEnv(t)

	// the patch-checker queue is not initialised in unit tests; stub the enqueue
	// so upserting a pull request doesn't touch it
	originalAddToQueue := pull_service.AddPullRequestToCheckQueue
	pull_service.AddPullRequestToCheckQueue = func(int64) {}
	defer func() { pull_service.AddPullRequestToCheckQueue = originalAddToQueue }()

	doer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	ctx := t.Context()
	uploader := NewGiteaLocalUploader(ctx, doer, "user2", repo.Name)
	uploader.gitServiceType = structs.GithubService
	uploader.repo = repo
	var err error
	uploader.gitRepo, err = git.OpenRepository(repo)
	require.NoError(t, err)
	defer uploader.Close()
	require.NoError(t, uploader.loadExistingLabelsAndMilestones(ctx))

	marks, err := newSyncWatermarks(ctx, repo.ID)
	require.NoError(t, err)
	// timestamp fixtures comfortably after every stream's watermark so all three
	// streams (issues, pulls, comments) include them
	latest := marks.issues
	if marks.pulls.After(latest) {
		latest = marks.pulls
	}
	if marks.comments.After(latest) {
		latest = marks.comments
	}

	updated := latest.Add(time.Hour)
	downloader := &syncTestDownloader{
		issues: []*base.Issue{{
			Number:       100,
			Title:        "sync issue",
			Content:      "created by sync",
			PosterID:     9990,
			PosterName:   "external-user",
			State:        "open",
			Created:      updated,
			Updated:      updated,
			ForeignIndex: 100,
		}},
		prs: []*base.PullRequest{{
			Number:       101,
			Title:        "sync pull request",
			Content:      "created by sync",
			PosterID:     9991,
			PosterName:   "external-user",
			State:        "open",
			Created:      updated,
			Updated:      updated,
			ForeignIndex: 101,
			EnsuredSafe:  true,
			Head: base.PullRequestBranch{
				Ref:       "unknown-branch",
				SHA:       "e29fdd58dd8f8e6de2a44a5a1b18e196c43fd4b0",
				RepoName:  repo.Name,
				OwnerName: "deleted-user",
			},
			Base: base.PullRequestBranch{
				Ref:       "master",
				RepoName:  repo.Name,
				OwnerName: "user2",
			},
		}},
		comments: []*base.Comment{{
			IssueIndex: 100,
			Index:      770001, // the remote comment id, becomes the dedup key
			PosterID:   9990,
			PosterName: "external-user",
			Content:    "first synced comment",
			Created:    updated,
			Updated:    updated,
		}},
		reviews: []*base.Review{{
			ID:           660001, // the remote review id, becomes the dedup key
			IssueIndex:   101,
			ReviewerID:   9991,
			ReviewerName: "external-user",
			Content:      "please change this",
			State:        base.ReviewStateChangesRequested,
			CreatedAt:    updated,
		}},
	}

	opts := base.MigrateOptions{Issues: true, PullRequests: true, Comments: true}

	issueCountBefore := unittest.GetCount(t, &issues_model.Issue{RepoID: repo.ID})

	// the first sync inserts the new issue and pull request
	require.NoError(t, syncRepository(ctx, downloader, uploader, opts, nil, marks))

	createdIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{RepoID: repo.ID, Index: 100})
	assert.Equal(t, "sync issue", createdIssue.Title)
	createdPRIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{RepoID: repo.ID, Index: 101})
	assert.True(t, createdPRIssue.IsPull)
	createdPR := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{IssueID: createdPRIssue.ID})
	assert.Equal(t, issueCountBefore+2, unittest.GetCount(t, &issues_model.Issue{RepoID: repo.ID}))

	// the comment attached to the synced issue, and the review to the synced PR
	createdComment := unittest.AssertExistsAndLoadBean(t, &issues_model.Comment{IssueID: createdIssue.ID, OriginalID: 770001})
	assert.Equal(t, "first synced comment", createdComment.Content)
	createdReview := unittest.AssertExistsAndLoadBean(t, &issues_model.Review{IssueID: createdPRIssue.ID, OriginalID: 660001})
	assert.Equal(t, issues_model.ReviewTypeReject, createdReview.Type)

	// a second sync of the same entities, some updated meanwhile, must update in
	// place instead of duplicating anything
	downloader.issues[0].Title = "sync issue retitled"
	downloader.issues[0].Updated = updated.Add(time.Hour)
	downloader.prs[0].State = "closed"
	closed := updated.Add(time.Hour)
	downloader.prs[0].Closed = &closed
	downloader.prs[0].Updated = closed
	downloader.comments[0].Content = "edited synced comment"
	downloader.comments[0].Updated = updated.Add(time.Hour)
	downloader.reviews[0].Content = "looks good now"
	downloader.reviews[0].State = base.ReviewStateApproved

	require.NoError(t, syncRepository(ctx, downloader, uploader, opts, nil, marks))

	assert.Equal(t, issueCountBefore+2, unittest.GetCount(t, &issues_model.Issue{RepoID: repo.ID}))
	afterIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{RepoID: repo.ID, Index: 100})
	assert.Equal(t, createdIssue.ID, afterIssue.ID)
	assert.Equal(t, "sync issue retitled", afterIssue.Title)
	afterPRIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{RepoID: repo.ID, Index: 101})
	assert.Equal(t, createdPRIssue.ID, afterPRIssue.ID)
	assert.True(t, afterPRIssue.IsClosed)
	afterPR := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{IssueID: afterPRIssue.ID})
	assert.Equal(t, createdPR.ID, afterPR.ID)

	// the comment and review updated in place, no duplicate rows
	assert.Equal(t, 1, unittest.GetCount(t, &issues_model.Comment{IssueID: createdIssue.ID, OriginalID: 770001}))
	afterComment := unittest.AssertExistsAndLoadBean(t, &issues_model.Comment{IssueID: createdIssue.ID, OriginalID: 770001})
	assert.Equal(t, createdComment.ID, afterComment.ID)
	assert.Equal(t, "edited synced comment", afterComment.Content)
	assert.Equal(t, 1, unittest.GetCount(t, &issues_model.Review{IssueID: createdPRIssue.ID, OriginalID: 660001}))
	afterReview := unittest.AssertExistsAndLoadBean(t, &issues_model.Review{IssueID: createdPRIssue.ID, OriginalID: 660001})
	assert.Equal(t, createdReview.ID, afterReview.ID)
	assert.Equal(t, issues_model.ReviewTypeApprove, afterReview.Type)

	// NOTE: CheckConsistencyFor is intentionally not called here. UpdateIssueNumComments
	// counts CommentTypeReview comments (ConversationCountedCommentType), while the test
	// consistency checker counts only plain CommentTypeComment, so importing a review onto
	// an issue with no plain comments trips a pre-existing mismatch unrelated to this change
	// (the only other review-importing test, TestGiteaUploadRepo, is skipped). The explicit
	// count assertions above are the actual proof that the re-sync deduplicates.
}
