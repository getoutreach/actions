// Copyright 2022 Outreach Corporation. All Rights Reserved.

// Description: This file defines the helper function that help parse common event
// payloads.

package gh

import (
	"encoding/json"

	"github.com/pkg/errors"
)

// PullRequest is a type meant for a pull_request payload to be marshaled into. This
// type can be extended with fields as they become necessary in actions.
//
// An example of all the fields that could be added to this type can be found in
// test/payloads/pull_request.json
type PullRequest struct {
	Title   string `json:"title"`
	Number  int    `json:"number"`
	Commits int    `json:"commits"`
	Head    struct {
		Ref string `json:"ref"` // HEAD branch name
		SHA string `json:"sha"` // SHA of last commit on HEAD
	} `json:"head"`
	Base struct {
		Repo struct {
			Name  string `json:"name"` // Repository name
			Ref   string `json:"ref"`  // Base branch name
			Owner struct {
				Login string `json:"login"` // Owner (organization/user) name
			} `json:"owner"`
		} `json:"repo"`
	} `json:"base"`
}

// ParsePullRequestPayload takes a GitHub actions payload and returns a *PullRequest
// type with the fields from the payload marshaled into the type.
func ParsePullRequestPayload(payload map[string]interface{}) (*PullRequest, error) {
	eventMap, ok := payload["pull_request"].(map[string]interface{})
	if !ok {
		return nil, errors.New("error asserting \"pull_request\" to map[string]interface{}")
	}

	b, err := json.Marshal(eventMap)
	if err != nil {
		return nil, errors.Wrap(err, "marshal event map into bytes")
	}

	var event PullRequest
	if err := json.Unmarshal(b, &event); err != nil {
		return nil, errors.Wrap(err, "unmarshal event map into concrete type")
	}

	return &event, nil
}

// Create is a type meant for a create payload to be marshaled into. This type can be
// extended with fields as they become necessary in actions.
//
// An example of all the fields that could be added to this type can be found in
// test/payloads/create.json
type Create struct {
	Ref        string `json:"ref"`
	RefType    string `json:"ref_type"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

// ParseCreatePayload takes a GitHub actions payload and returns a *Create type with the
// fields from the payload marshaled into the type.
func ParseCreatePayload(payload map[string]interface{}) (*Create, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal event map into bytes")
	}

	var event Create
	if err := json.Unmarshal(b, &event); err != nil {
		return nil, errors.Wrap(err, "unmarshal event map into concrete type")
	}

	return &event, nil
}

// Status is a type meant for a status payload to be marshaled into. This type can be
// extended with fields as they become necessary in actions.
//
// An example of all the fields that could be added to this type can be found in
// test/payloads/status_failure.json and test/payloads/status_success.json.
type Status struct {
	State     string  `json:"state"`
	Context   string  `json:"context"`
	TargetURL *string `json:"target_url"`
	Commit    struct {
		SHA     string `json:"SHA"`
		HTMLUrl string `json:"html_url"`
		Author  struct {
			Login   string `json:"login"`
			HTMLUrl string `json:"html_url"`
		} `json:"author"`
	} `json:"commit"`
	Branches []struct {
		Name string `json:"name"`
	} `json:"branches"`
	Repository struct {
		FullName string `json:"full_name"`
		HTMLUrl  string `json:"html_url"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

// ParseStatusPayload takes a GitHub actions payload and returns a *Status type with the
// fields from the payload marshaled into the type.
func ParseStatusPayload(payload map[string]interface{}) (*Status, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal event map into bytes")
	}

	var event Status
	if err := json.Unmarshal(b, &event); err != nil {
		return nil, errors.Wrap(err, "unmarshal event map into concrete type")
	}

	return &event, nil
}
