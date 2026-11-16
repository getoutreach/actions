// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: This file defines a client for the CircleCI API endpoints
// that rerun_e2e calls.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// circleciClient calls the CircleCI API.
type circleciClient struct {
	// baseURL is the URL the API paths are relative to, such as
	// https://circleci.com.
	baseURL string
	// token is the CircleCI API token sent in the Circle-Token header.
	token string
	http  *http.Client
}

// pipeline is a CircleCI pipeline.
type pipeline struct {
	ID        string    `json:"id"`
	Number    int       `json:"number"`
	CreatedAt time.Time `json:"created_at"`
	VCS       struct {
		Revision string `json:"revision"`
	} `json:"vcs"`
}

// workflow is a workflow in a CircleCI pipeline.
type workflow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// job is a job in a CircleCI workflow.
type job struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	// Number is the job number, or 0 when CircleCI hasn't assigned one.
	Number int `json:"job_number"`
}

// pipelines returns the first page of pipelines on a branch of the
// owner/repo GitHub project.
func (c *circleciClient) pipelines(ctx context.Context, owner, repo, branch string) ([]pipeline, error) {
	var page struct {
		Items []pipeline `json:"items"`
	}
	path := fmt.Sprintf("/api/v2/project/gh/%s/%s/pipeline", url.PathEscape(owner), url.PathEscape(repo))
	if err := c.do(ctx, http.MethodGet, path, url.Values{"branch": {branch}}, nil, &page); err != nil {
		return nil, err
	}
	return page.Items, nil
}

// workflows returns the workflows in a pipeline.
func (c *circleciClient) workflows(ctx context.Context, pipelineID string) ([]workflow, error) {
	return listAll[workflow](ctx, c, "/api/v2/pipeline/"+url.PathEscape(pipelineID)+"/workflow")
}

// jobs returns the jobs in a workflow.
func (c *circleciClient) jobs(ctx context.Context, workflowID string) ([]job, error) {
	return listAll[job](ctx, c, "/api/v2/workflow/"+url.PathEscape(workflowID)+"/job")
}

// stepNames returns the names of the steps that a job of the owner/repo
// GitHub project has started.
func (c *circleciClient) stepNames(ctx context.Context, owner, repo string, jobNumber int) ([]string, error) {
	var details struct {
		Steps []struct {
			Name string `json:"name"`
		} `json:"steps"`
	}
	path := fmt.Sprintf("/api/v1.1/project/github/%s/%s/%d", url.PathEscape(owner), url.PathEscape(repo), jobNumber)
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &details); err != nil {
		return nil, err
	}

	names := make([]string, len(details.Steps))
	for i := range details.Steps {
		names[i] = details.Steps[i].Name
	}
	return names, nil
}

// workflowStatus returns the status of a workflow.
func (c *circleciClient) workflowStatus(ctx context.Context, workflowID string) (string, error) {
	var wf workflow
	if err := c.do(ctx, http.MethodGet, "/api/v2/workflow/"+url.PathEscape(workflowID), nil, nil, &wf); err != nil {
		return "", err
	}
	return wf.Status, nil
}

// rerunJob re-runs a job of a finished workflow and returns the ID of the
// workflow that CircleCI creates for the re-run.
func (c *circleciClient) rerunJob(ctx context.Context, workflowID, jobID string) (string, error) {
	req := struct {
		Jobs       []string `json:"jobs"`
		SparseTree bool     `json:"sparse_tree"`
	}{Jobs: []string{jobID}, SparseTree: true}
	var resp struct {
		WorkflowID string `json:"workflow_id"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v2/workflow/"+url.PathEscape(workflowID)+"/rerun", nil, req, &resp); err != nil {
		return "", err
	}
	return resp.WorkflowID, nil
}

// listAll returns the items on every page of a CircleCI API v2 list
// endpoint.
func listAll[T any](ctx context.Context, c *circleciClient, path string) ([]T, error) {
	var items []T
	query := url.Values{}
	for {
		var page struct {
			Items         []T    `json:"items"`
			NextPageToken string `json:"next_page_token"`
		}
		if err := c.do(ctx, http.MethodGet, path, query, nil, &page); err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
		if page.NextPageToken == "" {
			return items, nil
		}
		query.Set("page-token", page.NextPageToken)
	}
}

// do sends a request to the CircleCI API, with reqBody as its JSON body
// unless it's nil, and decodes the JSON response into respBody.
func (c *circleciClient) do(ctx context.Context, method, path string, query url.Values, reqBody, respBody any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var body io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal %s %s request: %w", method, path, err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return fmt.Errorf("create %s %s request: %w", method, path, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Circle-Token", c.token)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s %s: %w", method, path, responseError(resp))
	}
	if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
		return fmt.Errorf("decode %s %s response: %w", method, path, err)
	}
	return nil
}

// responseError returns an error with the status of a failed CircleCI API
// response and the message in its body.
func responseError(resp *http.Response) error {
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%s: read body: %w", resp.Status, err)
	}

	var body struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(b, &body) == nil && body.Message != "" {
		return fmt.Errorf("%s: %s", resp.Status, body.Message)
	}
	return fmt.Errorf("%s: %s", resp.Status, bytes.TrimSpace(b))
}
