// Copyright 2022 Outreach Corporation. All Rights Reserved.

package main

import (
	"context"
	"os"
	"time"

	"github.com/getoutreach/actions/internal/gh"
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
	return errors.New("test is unimplemented")
}

