// Copyright 2022 Outreach Corporation. All Rights Reserved.

// Description: This file defines the thin wrapper around github.NewClient to deal with
// authentication.

// Package gh is a thin wrapper around the go-github client that handles authentication
// for the way we send the PAT to our action images. This package also contains helpful
// utilities for parsing common event payloads, like pull requests.
package gh

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/v43/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

// NewClient returns a new authenticated GitHub client using the token found at
// either GITHUB_TOKEN (local repository access) or PAT_OUTREACH_CI (org-wide
// access).
//
// If your action needs a client for org-wide access you must ensure that the
// caller of the action (from a GitHub workflow) writes the PAT_OUTREACH_CI secret
// to the PAT_OUTREACH_CI environment variable. For more information on how to
// configure an action to make org-wide requests, see the "Configuring an Action to
// Make Org-Wide GitHub API Requests" section in the README.md of this repository.
func NewClient(ctx context.Context, orgWideAccess bool) (*github.Client, error) {
	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if orgWideAccess {
		token = strings.TrimSpace(os.Getenv("PAT_OUTREACH_CI"))
	}

	if token == "" {
		return nil, errors.New("token from environment variable is empty")
	}

	return github.NewClient(oauth2.NewClient(ctx,
		oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: token,
			},
		)),
	), nil
}

// NewClientFromApp uses the github.com/bradleyfalzon/ghinstallation package to create an
// HTTP transport that utilizes the private key from an app passed in to provide access
// to the GitHub API. This should rarely ever be used, but is necessary for things like
// getting SAML identity using GitHub App credentials (an enterprise owner PAT is required
// otherwise). The GitHub App in this case needs administrative read permissions on the
// organization, but even with the permissions a normal PAT for the app will not work.
func NewClientFromApp(ctx context.Context, appID, installationID int64, privateKey []byte) (*github.Client, error) {
	gtr, err := ghinstallation.New(http.DefaultTransport, appID, installationID, privateKey)
	if err != nil {
		return nil, errors.Wrap(err, "create transport")
	}

	httpClient := http.DefaultClient
	httpClient.Transport = gtr

	return github.NewClient(httpClient), nil
}
