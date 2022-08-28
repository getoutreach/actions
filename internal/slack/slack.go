// Copyright 2022 Outreach Corporation. All Rights Reserved.

// Description: See package comment for this one-file package.

// Package slack is a package meant to help utilize the Slack API in GitHub actions.
package slack

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/slack-go/slack"
)

// NewClient returns a new slack client configured with the token we expect to exist in
// the environment.
func NewClient() (*slack.Client, error) {
	slackToken := strings.TrimSpace(os.Getenv("SLACK_TOKEN"))
	if slackToken == "" {
		return nil, errors.New("SLACK_TOKEN environment variable is empty")
	}
	return slack.New(slackToken), nil
}

// Hyperlink returns a hyperlink that be used in a slack message sent via the API.
func Hyperlink(text, link string) string {
	return fmt.Sprintf("<%s|%s>", link, text)
}

// Message returns a message able to be posted via slack.*Client.PostMessage[Context].
func Message(text string) slack.MsgOption {
	return slack.MsgOptionText(text, false)
}
