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

// FindChannelID given a channel name, finds the channel id.
func FindChannelID(channels []slack.Channel, name string) (string, error) {
	for i := range channels {
		if channels[i].Name == name {
			return channels[i].ID, nil
		}
	}

	return "", errors.New("could not find slack channel")
}

// GetAllChannels retrieves a list of all the available slack channels.
func GetAllChannels(client *slack.Client) ([]slack.Channel, error) {
	var channels []slack.Channel
	params := &slack.GetConversationsParameters{
		ExcludeArchived: true,
	}

	c, nextCursor, err := client.GetConversations(params)
	if err != nil {
		return nil, fmt.Errorf("get slack conversations: %w", err)
	}
	channels = c

	for nextCursor != "" {
		params.Cursor = nextCursor
		c, nextCursor, err = client.GetConversations(params)
		if err != nil {
			return nil, fmt.Errorf("get slack conversations: %w", err)
		}
		channels = append(channels, c...)
	}

	return channels, nil
}
