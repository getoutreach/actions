// Copyright 2022 Outreach Corporation. All Rights Reserved.

// Description: This file contains functions and types that help interacting with pull
// requests through the GitHub API.

package gh

import (
	"context"

	"github.com/google/go-github/v68/github"
	"github.com/pkg/errors"
)

// ListAllPullRequests list all pull requests for a given org/repo. State can be one of
// three values: "open", "closed", or "all".
func ListAllPullRequests(ctx context.Context, client *github.Client, org, repo, state string) ([]*github.PullRequest, error) {
	pullPage := 1
	pullsPerPage := 100

	var pulls []*github.PullRequest
	for pullPage != 0 {
		next, res, err := client.PullRequests.List(ctx, org, repo, &github.PullRequestListOptions{
			State: state,
			ListOptions: github.ListOptions{
				Page:    pullPage,
				PerPage: pullsPerPage,
			},
		})
		if err != nil {
			return nil, errors.Wrapf(err, "list %q pull requests", state)
		}

		pulls = append(pulls, next...)
		pullPage = res.NextPage
	}

	return pulls, nil
}
