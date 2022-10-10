// Copyright 2022 Outreach Corporation. All Rights Reserved.

package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/getoutreach/actions/pkg/gh"
	"github.com/getoutreach/actions/pkg/opslevel"
	"github.com/getoutreach/actions/pkg/slack"
	"github.com/google/go-github/v43/github"
	opslevelGo "github.com/opslevel/opslevel-go/v2022"
	"github.com/pkg/errors"
	actions "github.com/sethvargo/go-githubactions"
	slackGo "github.com/slack-go/slack"
)

// slackMessage is a template string of the slack message for each service.
//
//go:embed message.tpl
var slackMessage string

// SlackMessageFields holds the fields needed to execute the slack message template.
type SlackMessageFields struct {
	// MaturityReportHyperlink is a slack compliant hyperlink to the OpsLevel maturity report.
	MaturityReportHyperlink string
	// ActualLevel is the current level of the service.
	ActualLevel string
	// ExpectedLevel is the level that the service should be at.
	ExpectedLevel string
}

// slackMessageHeader is the header added to each slack message
const slackMessageHeader string = "Starting next quarter, deployments for these repositories will be blocked if they are not updated to meet their expected service maturity level in OpsLevel.\n" //nolint:lll // Why: Slack message string.

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
func RunAction(ctx context.Context, _ *github.Client, _ *actions.GitHubContext,
	slackClient *slackGo.Client, opslevelClient *opslevelGo.Client) error {
	t := template.Must(template.New("slackMessage").Parse(slackMessage))

	levels, err := opslevelClient.ListLevels()
	if err != nil {
		return fmt.Errorf("list levels: %w", err)
	}

	teams, err := opslevelClient.ListTeams()
	if err != nil {
		return fmt.Errorf("list teams: %w", err)
	}

	for i := range teams {
		team := &teams[i]
		slackMessage, err := buildSlackMessage(opslevelClient, team, levels, t)
		if err != nil {
			actions.Errorf("building slack message for %s: %v", team.Name, err.Error())
			continue
		}

		// If all services are compliant, we skip sending a slack message.
		if slackMessage == "" {
			continue
		}
		slackMessage = slackMessageHeader + slackMessage

		slackChannel, err := opslevel.GetSlackChannel(team)
		if err != nil {
			actions.Errorf("get slack channel for %s: %v", team.Name, err.Error())
			continue
		}

		channels, err := slack.GetAllChannels(slackClient)
		if err != nil {
			actions.Errorf("get channels for %s: %v", team.Name, err.Error())
			continue
		}

		slackChannelID, err := slack.FindChannelID(channels, slackChannel)
		if err != nil {
			actions.Errorf("find channel id for %s: %v", team.Name, err.Error())
			continue
		}

		if _, _, _, err := slackClient.JoinConversationContext(ctx, slackChannelID); err != nil {
			actions.Errorf("joining slack channel for %s: %v", team.Name, err.Error())
			continue
		}

		if _, _, err := slackClient.PostMessageContext(ctx, slackChannelID, slack.Message(slackMessage)); err != nil {
			actions.Errorf("posting slack message for %s: %v", team.Name, err.Error())
			continue
		}
	}
	return nil
}

// buildSlackMessage builds a slack message for all non compliant service that the provided team owns.
func buildSlackMessage(client *opslevelGo.Client, team *opslevelGo.Team,
	levels []opslevelGo.Level, t *template.Template) (string, error) {
	services, err := client.ListServicesWithOwner(team.Alias)
	if err != nil {
		return "", errors.Wrap(err, "could not list services")
	}

	var slackMessage strings.Builder
	for i := range services {
		service := &services[i]

		alias, err := opslevel.GetServiceAlias(service)
		if err != nil {
			actions.Errorf("get service alias for %s: %v", service.Name, err.Error())
			continue
		}

		sm, err := client.GetServiceMaturityWithAlias(alias)
		if err != nil {
			actions.Errorf("get maturity report for %s: %v", service.Name, err.Error())
			continue
		}

		isCompliant, err := opslevel.IsCompliant(service, sm)
		if err != nil {
			actions.Errorf("is compliant for %s: %v", service.Name, err.Error())
			continue
		}

		// If the service is compliant, we skip adding to the slack message.
		if isCompliant {
			continue
		}

		expectedLevel, err := opslevel.GetExpectedLevel(service, levels)
		if err != nil {
			actions.Errorf("get expected level for %s: %v", service.Name, err.Error())
			continue
		}

		if err := t.Execute(
			&slackMessage,
			SlackMessageFields{
				ActualLevel:   opslevel.GetLevel(sm),
				ExpectedLevel: expectedLevel,
				MaturityReportHyperlink: slack.Hyperlink(service.Name,
					opslevel.GetMaturityReportURL(service)),
			},
		); err != nil {
			actions.Errorf("building slack message for %s: %v", service.Name, err.Error())
			continue
		}
	}

	return slackMessage.String(), nil
}
