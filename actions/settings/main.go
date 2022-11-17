// Copyright 2022 Outreach Corporation. All Rights Reserved.

package main

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/getoutreach/actions/pkg/gh"
	"github.com/google/go-github/v43/github"
	"github.com/pkg/errors"
	actions "github.com/sethvargo/go-githubactions"
)

func main() {
	exitCode := 1
	defer func() {
		os.Exit(exitCode)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	client, err := gh.NewClient(ctx, false)
	if err != nil {
		actions.Errorf("create github client: %v", err)
		return
	}

	ghContext, err := actions.Context()
	if err != nil {
		actions.Errorf("unable to get action context: %v", err)
		return
	}

	if err := RunAction(ctx, client, ghContext); err != nil {
		actions.Errorf(err.Error())
		return
	}
	exitCode = 0
}

// RunAction is where the actual implementation of the GitHub action goes and is called
// by func main.
func RunAction(ctx context.Context, client *github.Client, actionCtx *actions.GitHubContext) error {
	owner := "owner"
	repo := "repo"
	defaultBranch := "defaultBranch"

	allowRebaseMerge, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("ALLOW_REBASE_MERGE")))
	if err != nil {
		return errors.Wrap(err, "parse boolean from ALLOW_REBASE_MERGE")
	}

	allowSquashMerge, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("ALLOW_SQUASH_MERGE")))
	if err != nil {
		return errors.Wrap(err, "parse boolean from ALLOW_SQUASH_MERGE")
	}

	allowMergeCommit, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("ALLOW_MERGE_COMMIT")))
	if err != nil {
		return errors.Wrap(err, "parse boolean from ALLOW_MERGE_COMMIT")
	}

	allowAutoMerge, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("ALLOW_AUTO_MERGE")))
	if err != nil {
		return errors.Wrap(err, "parse boolean from ALLOW_AUTO_MERGE")
	}

	actions.Infof("updating repository (%s/%s) settings:\n * AllowRebaseMerge: %t\n * AllowSquashMerge: %t\n * AllowMergeCommit: %t\n * AllowAutoMerge: %t", //nolint:lll Why: Log formatted string.
		owner, repo, allowRebaseMerge, allowSquashMerge, allowMergeCommit, allowAutoMerge)

	_, _, err = client.Repositories.Edit(ctx, owner, repo, &github.Repository{
		AllowRebaseMerge: &allowRebaseMerge,
		AllowSquashMerge: &allowSquashMerge,
		AllowMergeCommit: &allowMergeCommit,
		AllowAutoMerge:   &allowAutoMerge,
	})

	if err != nil {
		return errors.Wrap(err, "update repository settings")
	}

	requiredConversationResolution, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("REQUIRED_CONVERSATION_RESOLUTION")))
	if err != nil {
		return errors.Wrap(err, "parse boolean from REQUIRED_CONVERSATION_RESOLUTION")
	}

	requireCodeOwnerReviewers, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("REQUIRE_CODEOWNER_REVIEWERS")))
	if err != nil {
		return errors.Wrap(err, "parse boolean from REQUIRE_CODEOWNER_REVIEWERS")
	}

	requiredApprovingReviewCount, err := strconv.Atoi(strings.TrimSpace(os.Getenv("REQUIRED_APPROVING_REVIEW_COUNT")))
	if err != nil {
		return errors.Wrap(err, "parse int from REQUIRED_APPROVING_REVIEW_COUNT")
	}

	actions.Infof("updating repository (%s/%s) branch () protection rules:\n * RequireCodeOwnerReviews: %t\n * RequiredApprovingReviewCount: %d\n * RequiredConversationResolution: %t", //nolint:lll Why: Log formatted string.
		owner, repo, defaultBranch, requireCodeOwnerReviewers, requiredApprovingReviewCount, requiredConversationResolution)

	_, _, err = client.Repositories.UpdateBranchProtection(ctx, owner, repo, defaultBranch, &github.ProtectionRequest{
		RequiredPullRequestReviews: &github.PullRequestReviewsEnforcementRequest{
			RequireCodeOwnerReviews:      requireCodeOwnerReviewers,
			RequiredApprovingReviewCount: requiredApprovingReviewCount,
		},
		RequiredConversationResolution: &requiredConversationResolution,
	})

	if err != nil {
		return errors.Wrap(err, "update branch protection rules")
	}

	requiredStatusChecks := strings.Split(strings.TrimSpace(os.Getenv("REQUIRED_STATUS_CHECKS")), ",")

	checks := make([]*github.RequiredStatusCheck, 0, len(requiredStatusChecks))
	for i := range requiredStatusChecks {
		checks = append(checks, &github.RequiredStatusCheck{
			Context: strings.TrimSpace(requiredStatusChecks[i]),
		})
	}

	actions.Infof("updating repository (%s/%s) branch (%s) required status checks: %+v",
		owner, repo, defaultBranch, requiredStatusChecks)

	_, _, err = client.Repositories.UpdateRequiredStatusChecks(ctx, owner, repo, defaultBranch, &github.RequiredStatusChecksRequest{
		Checks: checks,
	})

	if err != nil {
		return errors.Wrap(err, "update required status checks")
	}

	return nil
}
