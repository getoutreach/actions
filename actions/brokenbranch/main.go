// Copyright 2022 Outreach Corporation. All Rights Reserved.

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/getoutreach/actions/internal/gh"
	"github.com/getoutreach/actions/internal/slack"
	"github.com/google/go-github/v43/github"
	"github.com/pkg/errors"
	actions "github.com/sethvargo/go-githubactions"
	_slack "github.com/slack-go/slack"
)

// Constant block for slack message formatted strings.
const (
	// slackChannelMessageFmt is a formatted string that is meant to have the following data
	// passed to it to build it:
	//	- Branch name
	//	- Hyperlinked Repository
	//	- Context (name of failing step), hyperlinked if targetURL exists.
	//	- Hyperlinked Commit SHA
	//	- Hyperlinked Commit Author
	//	- Appended message content (anything, or empty)
	//
	// All of this information can be found in the status event payload sent to the action.
	slackChannelMessageFmt = `Looks like the build on branch ` + "`%s`" + ` in *%s* is broken.
---
Check: *%s*
Commit: *%s*
Committer: *%s*%s`

	// slackDMMessageFmt is a formatted string that is meant to have the following data passed
	// to it to build it:
	//	- The word "commit" hyperlinked to the failing commit.
	//	- Branch name
	//	- Hyperlinked Repository
	//	- Context (name of failing step), hyperlinked if targetURL exists.
	//
	// All of this information can be found in the status event payload sent to the action.
	slackDMMessageFmt = `Looks like you pushed a %s on branch ` + "`%s`" + ` in *%s* that failed the *%s* check. Please go address this.` //nolint:lll // Why: Slack message string.
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
func RunAction(ctx context.Context, _ *github.Client, actionCtx *actions.GitHubContext) error {
	if actionCtx.EventName != "status" {
		return errors.New("brokenbranch running on non \"status\" event")
	}

	ignoredChecksRaw := strings.TrimSpace(os.Getenv("IGNORED_CHECKS"))
	ignoredChecks := strings.Split(ignoredChecksRaw, ",")
	for i := range ignoredChecks {
		ignoredChecks[i] = strings.TrimSpace(ignoredChecks[i])
	}

	githubBranch := strings.TrimSpace(os.Getenv("GITHUB_BRANCH"))
	slackChannel := strings.TrimSpace(os.Getenv("SLACK_CHANNEL"))
	dmCommitter := strings.TrimSpace(os.Getenv("DM_COMMITTER"))
	ghAppID := strings.TrimSpace(os.Getenv("GH_APP_ID"))
	ghAppInstallationID := strings.TrimSpace(os.Getenv("GH_APP_INSTALLATION_ID"))
	ghAppPrivateKeyBase64 := strings.TrimSpace(os.Getenv("GH_APP_PRIVATE_KEY_BASE64"))

	if dmCommitter == "true" &&
		(ghAppID == "" || ghAppInstallationID == "" || ghAppPrivateKeyBase64 == "") {
		return errors.New("GH_APP_ID, GH_APP_INSTALLATION_ID, and GH_APP_PRIVATE_KEY_BASE64 are all required if dm_committer input is set to true") //nolint:lll // Why: Just an error string.
	}

	if githubBranch == "" {
		return errors.New("GITHUB_BRANCH environment variable is empty")
	}

	if slackChannel == "" {
		return errors.New("SLACK_CHANNEL environment variable is empty")
	}

	status, err := gh.ParseStatusPayload(actionCtx.Event)
	if err != nil {
		return errors.Wrap(err, "parse event payload")
	}

	if status.State != "failure" {
		actions.Infof("status state not failure, skipping")
		return nil
	}

	for i := range ignoredChecks {
		if strings.EqualFold(status.Context, ignoredChecks[i]) {
			actions.Infof("failed check (%s) is in ignored_checks input, skipping", status.Context)
			return nil
		}
	}

	var foundBranch bool
	for i := range status.Branches {
		if status.Branches[i].Name == githubBranch {
			foundBranch = true
			break
		}
	}

	if !foundBranch {
		actions.Infof("did not find branch %q in status event payload", githubBranch)
	}

	slackClient, err := slack.NewClient()
	if err != nil {
		return errors.Wrap(err, "create slack client")
	}

	failedCheck := status.Context
	if status.TargetURL != nil {
		failedCheck = slack.Hyperlink(failedCheck, *status.TargetURL)
	}

	hyperlinkedRepository := slack.Hyperlink(status.Repository.FullName, status.Repository.HTMLUrl)
	hyperlinkedCommitSHA := slack.Hyperlink(status.Commit.SHA, status.Commit.HTMLUrl)

	hyperlinkedCommitter := status.Commit.Author.Login
	if status.Commit.Author.HTMLUrl != "" {
		hyperlinkedCommitter = slack.Hyperlink(status.Commit.Author.Login, status.Commit.Author.HTMLUrl)
	}

	var dmCommitterErr error
	if dmCommitter == "true" {
		slackDMMessage := fmt.Sprintf(slackDMMessageFmt,
			slack.Hyperlink("commit", status.Commit.HTMLUrl), githubBranch, hyperlinkedRepository, failedCheck)
		dmCommitterErr = messageCommitter(ctx, slackClient, ghAppID, ghAppInstallationID, ghAppPrivateKeyBase64,
			status.Repository.Owner.Login, status.Commit.Author.Login, slackDMMessage)
	}

	var appendToChannelMessage string
	if dmCommitterErr != nil {
		// append to channel message
		appendToChannelMessage = fmt.Sprintf(`
		
:warning: *Was unable to DM the committer* due to error: %s`, dmCommitterErr.Error())
	}

	slackChannelMessage := fmt.Sprintf(slackChannelMessageFmt,
		githubBranch, hyperlinkedRepository, failedCheck, hyperlinkedCommitSHA, hyperlinkedCommitter, appendToChannelMessage)

	if _, _, err := slackClient.PostMessageContext(ctx, slackChannel, slack.Message(slackChannelMessage)); err != nil {
		return errors.Wrap(err, "post message to channel")
	}

	return nil
}

// messageCommitter works by resolving a committers slack identity via their SAML identity.
// This is likely tied together via their organization email. This requires GitHub App
// auth via private key using an app that has administrative read rights. The PAT for
// an app with these rights will not work - it has to use a private key for some reason.
//
// The reason this function has so many parameters as opposed to breaking it up into smaller
// functions is because it makes the handling logic in RunAction much simpler if this is
// all self-contained.
func messageCommitter(ctx context.Context, slackClient *_slack.Client,
	appID, appInstallationID, appPrivateKeyBase64,
	orgLogin, committerLogin, message string) error {
	parsedAppPrivateKey, err := base64.StdEncoding.DecodeString(appPrivateKeyBase64)
	if err != nil {
		return errors.Wrap(err, "parse GH_APP_PRIVATE_KEY_BASE_64")
	}

	parsedAppID, err := strconv.Atoi(appID)
	if err != nil {
		return errors.Wrap(err, "parse GH_APP_ID")
	}

	parsedAppInstallationID, err := strconv.Atoi(appInstallationID)
	if err != nil {
		return errors.Wrap(err, "parse GH_APP_INSTALLATION_ID")
	}

	ghAppClient, err := gh.NewClientFromApp(ctx, int64(parsedAppID), int64(parsedAppInstallationID), parsedAppPrivateKey)
	if err != nil {
		return errors.Wrap(err, "create github client for app")
	}

	identity, err := gh.RetrieveSAMLIdentity(ctx, ghAppClient, orgLogin, committerLogin)
	if err != nil {
		return errors.Wrap(err, "retrieve SAML identity of committer from github graphql API")
	}

	slackUser, err := slackClient.GetUserByEmail(identity)
	if err != nil {
		return errors.Wrap(err, "retrieve slack identity of committer from SAML identity email")
	}

	channel, _, _, err := slackClient.OpenConversation(&_slack.OpenConversationParameters{
		Users: []string{slackUser.ID},
	})
	if err != nil {
		return errors.Wrap(err, "open slack conversation with committer")
	}

	if _, _, err := slackClient.PostMessageContext(ctx, channel.ID, slack.Message(message)); err != nil {
		return errors.Wrap(err, "send slack message to committer")
	}

	return nil
}
