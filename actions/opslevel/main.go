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
OpsLevel Maturity Report: *%s*
%s`
)

func main() {
	exitCode := 1
	defer func() {
		os.Exit(exitCode)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*60)
	defer cancel()

	client, err := gh.NewClient(ctx, false)
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

	if err := RunAction(ctx, client, ghContext, slackClient, opslevelClient); err != nil {
		actions.Errorf(err.Error())
		return
	}
	exitCode = 0
}

// RunAction is where the actual implementation of the GitHub action goes and is called
// by func main.
func RunAction(ctx context.Context, _ *github.Client, _ *actions.GitHubContext, //nolint:funlen // Why: runs the action
	slackClient *slackGo.Client, opslevelClient *opslevelGo.Client) error {
	levels, err := opslevelClient.ListLevels()
	if err != nil {
		return errors.Wrap(err, "could not list levels")
	}

	channels, err := slack.GetAllChannels(slackClient)
	if err != nil {
		return errors.Wrap(err, "could not get slack channels")
	}

	services, err := opslevelClient.ListServices()
	if err != nil {
		return errors.Wrap(err, "could not list services")
	}

	for i := range services {
		service := &services[i]

		alias, err := opslevel.GetServiceAlias(service)
		if err != nil {
			actions.Errorf("get service alias for %s: %v", service.Name, err.Error())
			continue
		}

		sm, err := opslevelClient.GetServiceMaturityWithAlias(alias)
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
		// We need to drop the leading `#`.
		// This is safe to do with index because it is known to always equal `#`.
		slackChannel = slackChannel[1:]

		slackChannelID, err := slack.FindChannelID(channels, slackChannel)
		if err != nil {
			actions.Errorf("find channel id for %s: %v", service.Name, err.Error())
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
			slack.Hyperlink("Maturity Report", opslevel.GetMaturityReportURL(service)),
			`Starting next quarter, this repository will no longer be able to deploy.
Please update it to the specified maturity level`,
		)

		if _, _, _, err := slackClient.JoinConversationContext(ctx, slackChannelID); err != nil {
			actions.Errorf("joining slack channel for %s: %v", service.Name, err.Error())
			continue
		}

		if _, _, err := slackClient.PostMessageContext(ctx, slackChannelID, slack.Message(slackMessage)); err != nil {
			actions.Errorf("posting slack message for %s: %v", service.Name, err.Error())
			continue
		}
	}

	return nil
}
