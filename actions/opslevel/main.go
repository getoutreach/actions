// Copyright 2022 Outreach Corporation. All Rights Reserved.

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/getoutreach/actions/internal/gh"
	"github.com/getoutreach/actions/internal/opslevel"
	"github.com/getoutreach/actions/internal/slack"
	"github.com/google/go-github/v43/github"
	opslevelGo "github.com/opslevel/opslevel-go/v2022"
	"github.com/pkg/errors"
	actions "github.com/sethvargo/go-githubactions"
	slackGo "github.com/slack-go/slack"
)

// Constant block for slack message formatted strings.
const (
	// slackMessageFmt is a formatted string that is meant to have the following data
	// passed to it to build it:
	//	- Project repository hyperlink
	//      - Expected OpsLevel level
	//      - Actual OpsLevel level
	//	- Hyperlinked OpsLevel Maturity Report
	//	- Appended message content (anything, or empty)
	//
	// All of this information can be found in the status event payload sent to the action.
	slackMessageFmt = `Looks like` + "`%s`" + ` does not meet the specied level in OpsLevel.
---
Expected Level: *%s*
Actual Level: *%s*
OpsLevel Maturity Report: *%s*%s`
)

func main() {
	exitCode := 1
	defer func() {
		os.Exit(exitCode)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*30)
	defer cancel()

	client, err := gh.NewClient(ctx, true)
	if err != nil {
		actions.Errorf("create github client: %v", err)
		return
	}

	ghContext, err := actions.Context()
	if err != nil {
		actions.Errorf("unable to get action context: %v", err)
		return
	}

	slackClient, err := slack.NewClient()
	if err != nil {
		actions.Errorf("create slack client: %v", err)
		return
	}

	opslevelClient, err := opslevel.NewClient()
	if err != nil {
		actions.Errorf("create opslevel client: %v", err)
		return
	}

	fmt.Printf("Trying to run")
	if err := RunAction(ctx, client, ghContext, slackClient, opslevelClient); err != nil {
		actions.Errorf(err.Error())
		return
	}
	exitCode = 0
}

// RunAction is where the actual implementation of the GitHub action goes and is called
// by func main.
func RunAction(ctx context.Context, _ *github.Client, _ *actions.GitHubContext,
	slackClient *slackGo.Client, opslevelClient *opslevelGo.Client) error {
	services, err := opslevelClient.ListServices()
	if err != nil {
		return errors.Wrap(err, "could not list services")
	}

	for i := range services {
		service, err := opslevel.NewService(opslevelClient, services[i].Aliases[0])
		if err != nil {
			actions.Errorf(err.Error())
			continue
		}

		isCompliant, err := service.IsCompliant()
		if err != nil {
			actions.Errorf(err.Error())
			continue
		}
		// if the repository is compliant we do not do anything
		if isCompliant {
			continue
		}

		repositoryHyperlink, err := service.GetRepositoryHyperlink()
		if err != nil {
			actions.Errorf(err.Error())
			continue
		}

		expectedLevel, err := service.GetExpectedLevel()
		if err != nil {
			actions.Errorf(err.Error())
			continue
		}

		level, err := service.GetLevel()
		if err != nil {
			actions.Errorf(err.Error())
			continue
		}

		maturityReportHyperlink := service.GetMaturityReportHyperlink()

		slackMessage := fmt.Sprintf(
			slackMessageFmt,
			repositoryHyperlink,
			expectedLevel,
			level,
			maturityReportHyperlink,
			`Starting next quarter, this repository will no longer be able to deploy.
Please update it to the specified maturity level`,
		)

		slackChannel, err := service.GetSlackChannel()
		if err != nil {
			actions.Errorf(err.Error())
			continue
		}

		if _, _, err := slackClient.PostMessageContext(ctx, slackChannel, slack.Message(slackMessage)); err != nil {
			actions.Errorf(err.Error())
			continue
		}
	}

	return nil
}
