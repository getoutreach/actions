// Copyright 2022 Outreach Corporation. All Rights Reserved.

// Description: See package comment for this one-file package.

// Package slack is a package meant to help utilize the Slack API in GitHub actions.
package slack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

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

// GetAllChannels retrieves a list of all the available slack channels and retries once if needed.
func GetAllChannels(client *slack.Client) ([]slack.Channel, error) {
	var channels []slack.Channel
	params := &slack.GetConversationsParameters{
		ExcludeArchived: true,
	}

	c, nextCursor, err := client.GetConversations(params)
	if err != nil {
		var rateLimitedError *slack.RateLimitedError
		if errors.As(err, &rateLimitedError) {
			if rateLimitedError.Retryable() {
				fmt.Println("retrying get slack conversations")
				time.Sleep(rateLimitedError.RetryAfter)
				c, nextCursor, err = client.GetConversations(params)
			} else {
				return nil, fmt.Errorf("get slack conversations: %w", err)
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("get slack conversations: %w", err)
	}
	channels = c

	for nextCursor != "" {
		params.Cursor = nextCursor

		c, nextCursor, err = client.GetConversations(params)
		if err != nil {
			var rateLimitedError *slack.RateLimitedError
			if errors.As(err, &rateLimitedError) {
				if rateLimitedError.Retryable() {
					fmt.Println("retrying get slack conversations")
					time.Sleep(rateLimitedError.RetryAfter)
					c, nextCursor, err = client.GetConversations(params)
				} else {
					return nil, fmt.Errorf("get slack conversations: %w", err)
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("get slack conversations: %w", err)
		}
		channels = append(channels, c...)
	}

	return channels, nil
}

// JoinConversationContext joins the specified slack channel and retries once if needed.
func JoinConversationContext(ctx context.Context, client *slack.Client, channelID string) error {
	if _, _, _, err := client.JoinConversationContext(ctx, channelID); err != nil {
		var rateLimitedError *slack.RateLimitedError
		if errors.As(err, &rateLimitedError) {
			if rateLimitedError.Retryable() {
				fmt.Println("retrying join slack conversation")
				time.Sleep(rateLimitedError.RetryAfter)
				_, _, _, err = client.JoinConversationContext(ctx, channelID)
				return err
			}
		}
		return err
	}
	return nil
}

// PostMessageContext posts a message to the specified channel and retries if needed. Before trying to
// post, you need to make sure that you are part of channel by using JoinConversationContext.
func PostMessageContext(ctx context.Context, client *slack.Client, channelID, message string) error {
	if _, _, err := client.PostMessageContext(ctx, channelID, Message(message)); err != nil {
		var rateLimitedError *slack.RateLimitedError
		if errors.As(err, &rateLimitedError) {
			if rateLimitedError.Retryable() {
				fmt.Println("retrying post slack message")
				time.Sleep(rateLimitedError.RetryAfter)
				_, _, err = client.PostMessageContext(ctx, channelID)
				return err
			}
		}
	}
	return nil
}
