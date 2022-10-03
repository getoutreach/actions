// Copyright 2022 Outreach Corporation. All Rights Reserved.

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/getoutreach/actions/internal/gh"
	"github.com/getoutreach/actions/internal/slack"
	"github.com/getoutreach/actions/pkg/opslevel"
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
	slackMessageFmt = `Your service ` + "`%s`" + ` does not meet the required level in OpsLevel.
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
		actions.Errorf("get action context: %v", err)
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

	levels, err := opslevelClient.ListLevels()
	if err != nil {
		return errors.Wrap(err, "could not list levels")
	}

	service, err := opslevelClient.GetServiceWithAlias("devtooltestservice")
	if err != nil {
		return errors.Wrap(err, "could get service")
	}

	services = []opslevelGo.Service{*service}

	for i := range services {
		service := &services[i]

		sm, err := opslevelClient.GetServiceMaturityWithAlias(service.Aliases[0])
		if err != nil {
			actions.Errorf("get maturity report for %s: %v", service.Name, err.Error())
			continue
		}

		isCompliant, err := opslevel.IsCompliant(service, sm)
		if err != nil {
			actions.Errorf("is complient for %s: %v", service.Name, err.Error())
			continue
		}

		if isCompliant {
			continue
		}

		repoID, err := opslevel.GetRepositoryID(service)
		if err != nil {
			actions.Errorf("get repository id for %s: %v", service.Name, err.Error())
			continue
		}

		repo, err := opslevelClient.GetRepository(repoID)
		if err != nil {
			actions.Errorf("get repository for %s: %v", service.Name, err.Error())
			continue
		}

		expectedLevel, err := opslevel.GetExpectedLevel(service, levels)
		if err != nil {
			actions.Errorf("get expected level for %s: %v", service.Name, err.Error())
			continue
		}

		slackMessage := fmt.Sprintf(
			slackMessageFmt,
			slack.Hyperlink(service.Name, repo.Url),
			expectedLevel,
			opslevel.GetLevel(sm),
			opslevel.GetMaturityReportURL(service),
			`Starting next quarter, this repository will no longer be able to deploy.
Please update it to the specified maturity level`,
		)

		team, err := opslevelClient.GetTeam(service.Owner.Id)
		if err != nil {
			actions.Errorf("get team for %s: %v", service.Name, err.Error())
			continue
		}

		slackChannel, err := opslevel.GetSlackChannel(team)
		if err != nil {
			actions.Errorf("get slack channel for %s: %v", service.Name, err.Error())
			continue
		}

		fmt.Printf("got channel: %s\n", slackChannel)

		slackChannel = "dev-tooling-support-test"

		if _, _, err := slackClient.PostMessageContext(ctx, slackChannel, slack.Message(slackMessage)); err != nil {
			actions.Errorf("posting slack message for %s: %v", service.Name, err.Error())
			continue
		}
	}

	return nil
}
