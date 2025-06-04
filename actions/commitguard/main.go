// Copyright 2022 Outreach Corporation. All Rights Reserved.

package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/getoutreach/actions/pkg/gh"
	"github.com/google/go-github/v72/github"
	"github.com/pkg/errors"
	actions "github.com/sethvargo/go-githubactions"
)

// pullRequestSharedActionsWorkflowFile is the name of the shared workflow file that
// contains workflows triggered from pull requests. This is the name of the workflow
// file that should contain the job that runs CommitGuard on pull requests. We use
// this to attempt to re-run CommitGuard runs on open pull requests when a new tag
// has been pushed.
//
// While this doesn't seem ideal, this is just how the GitHub API works and we have to
// conform to it.
const pullRequestSharedActionsWorkflowFile = "pull_request-shared-actions.yaml"

// tagPrefix is the tag prefix this bot looks for in order to determine the
// commit that must exist in the history of a pull request's commits.
//
// The bot will ignore case when looking for this prefix in a repositories tags.
//
// Here is how you would create a tag in your repository for the CommitGuard:
//
//	CM_TAG_NAME="CommitGuard-$(date +%s)"
//	git tag -a $CM_TAG_NAME
//	git push origin $CM_TAG_NAME
//	unset CM_TAG_NAME
//
// Make sure you give your created tag a good description as to why that place in
// history is important to your repository.
const tagPrefix = "commitguard-"

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
		actions.Errorf("%s", err.Error())
		return
	}
	exitCode = 0
}

// RunAction is where the actual implementation of the GitHub action goes and is called
// by func main.
func RunAction(ctx context.Context, client *github.Client, actionCtx *actions.GitHubContext) error {
	switch en := actionCtx.EventName; en {
	case "pull_request":
		return runOnPullRequest(ctx, client, actionCtx)
	case "create":
		return runOnCreate(ctx, client, actionCtx)
	default:
		return fmt.Errorf("unknown event type %q", en)
	}
}

// runOnCreate reruns all CommitGuard checks on all open pull requests whenever a create event triggers
// this action. A create event happens when a branch or tag is pushed, although we only care about tag
// pushes (for new CommitGuard tags).
func runOnCreate(ctx context.Context, client *github.Client, actionCtx *actions.GitHubContext) error {
	create, err := gh.ParseCreatePayload(actionCtx.Event)
	if err != nil {
		return errors.Wrap(err, "parse event payload")
	}

	if create.RefType != "tag" || !strings.HasPrefix(strings.ToLower(create.Ref), tagPrefix) {
		// We only care about new CommitGuard tag pushes, ignore whatever this is.
		return nil
	}

	pulls, err := gh.ListAllPullRequests(ctx, client, create.Repository.Owner.Login, create.Repository.Name, "open")
	if err != nil {
		return errors.Wrap(err, "list all open pull requests")
	}

	if len(pulls) == 0 {
		// There are no CommitGuard checks to rerun.
		return nil
	}

	// Transform the open pull requests into a map that way it makes finding them easier
	// below when we're trying to figure out which workflows still have open pull requests
	// so they can be reran.
	openPulls := make(map[int]struct{})
	for i := range pulls {
		if pulls[i].Number == nil {
			continue
		}
		openPulls[*pulls[i].Number] = struct{}{}
	}

	var workflowsToRerun []int64

	workflowPage := 1
	for workflowPage != 0 {
		workflowRuns, res, err := client.Actions.ListWorkflowRunsByFileName(ctx, create.Repository.Owner.Login, create.Repository.Name,
			pullRequestSharedActionsWorkflowFile, &github.ListWorkflowRunsOptions{
				Event: "pull_request",
				ListOptions: github.ListOptions{
					Page: workflowPage,
				},
			})

		if err != nil {
			return errors.Wrapf(err, "list all workflow runs for %q", pullRequestSharedActionsWorkflowFile)
		}

		for _, workflowRun := range workflowRuns.WorkflowRuns {
			if workflowRun.ID == nil {
				continue
			}

			for _, pullRequest := range workflowRun.PullRequests {
				if pullRequest.Number == nil {
					continue
				}

				if _, exists := openPulls[*pullRequest.Number]; exists {
					// Pull request is still open, that means this workflow should be reran.
					workflowsToRerun = append(workflowsToRerun, *workflowRun.ID)
				}
			}
		}

		workflowPage = res.NextPage
	}

	for i := range workflowsToRerun {
		actions.Infof("rerunning workflow id %d", workflowsToRerun[i])
		if _, err := client.Actions.RerunWorkflowByID(ctx, create.Repository.Owner.Login, create.Repository.Name, workflowsToRerun[i]); err != nil { //nolint:lll //Why: Inline error.
			actions.Warningf("error rerunning workflow with id %d: %s", workflowsToRerun[i], err.Error())
		}
	}

	return nil
}

// runOnPullRequest runs CommitGuard on a pull_request event. This is the "normal" path.
func runOnPullRequest(ctx context.Context, client *github.Client, actionCtx *actions.GitHubContext) error {
	pr, err := gh.ParsePullRequestPayload(actionCtx.Event)
	if err != nil {
		return errors.Wrap(err, "parse event payload")
	}

	requiredSHA, err := findCommitGuardRequiredSHA(ctx, client, pr.Base.Repo.Owner.Login, pr.Base.Repo.Name)
	if err != nil {
		return errors.Wrap(err, "get required commit sha from inspecting tags")
	}

	actions.Infof("parsed necessary information:\nbranch: [%s]\nrequired commit sha: [%s]", pr.Head.Ref, requiredSHA)

	if requiredSHA == "" {
		actions.Infof("no CommitGuard tags found, skipping check")
		return nil
	}

	// What is happening here is explain in this stackoverflow answer, specifically
	// "Workaround 2": https://stackoverflow.com/a/23970412
	comparison, _, err := client.Repositories.CompareCommits(ctx, pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pr.Head.Ref, requiredSHA, nil)
	if err != nil {
		return errors.Wrap(err, "call to github api to compare head ref to required commit failed")
	}

	if status := comparison.GetStatus(); status == "diverged" || status == "ahead" {
		actions.Infof("comparison status: [%s]", status)
		return errors.New("branch does not contain required commit sha, please rebase")
	}
	actions.Infof("branch contains required commit")

	return nil
}

// findCommitGuardRequiredSHA looks through a repositories tags to get the most recent CommitGuard
// tag's corresponding commit SHA.
func findCommitGuardRequiredSHA(ctx context.Context, client *github.Client, org, repo string) (string, error) {
	tagPage := 1
	tagsPerPage := 100

	var requiredCommitSHA string
	var mostRecentTimestamp int

	for tagPage != 0 {
		tags, res, err := client.Repositories.ListTags(ctx, org, repo, &github.ListOptions{
			PerPage: tagsPerPage,
			Page:    tagPage,
		})
		if err != nil {
			return "", errors.Wrapf(err, "list tags page %d", tagPage)
		}

		for i := range tags {
			tag := tags[i]

			if tag.Name == nil || tag.Commit == nil || tag.Commit.SHA == nil {
				continue
			}

			if strings.HasPrefix(strings.ToLower(*tag.Name), tagPrefix) {
				stringTimestamp := strings.TrimPrefix(strings.ToLower(*tag.Name), tagPrefix)

				currentTimestamp, err := strconv.Atoi(stringTimestamp) //nolint:govet // Why: It's okay to shadow the error variable here.
				if err != nil {
					continue
				}

				if currentTimestamp > mostRecentTimestamp {
					mostRecentTimestamp = currentTimestamp
					requiredCommitSHA = *tag.Commit.SHA
				}
			}
		}

		// Set it to the next page. Once we're done paginating this value should be zero which
		// the loop will recognize and exit.
		tagPage = res.NextPage
	}

	return requiredCommitSHA, nil
}
