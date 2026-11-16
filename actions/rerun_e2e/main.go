// Copyright 2026 Outreach Corporation. All Rights Reserved.

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/getoutreach/actions/pkg/gh"
	actions "github.com/sethvargo/go-githubactions"
)

// runTestsStep is the name of the step in devbase's shared/e2e CircleCI job
// that runs the E2E tests. CircleCI only lists the steps a job has started, so
// a finished job without this step was halted before its tests.
const runTestsStep = "Run E2E Tests"

// pollInterval is how long to wait between looks at the E2E job.
const pollInterval = 30 * time.Second

// timeout bounds a run. It's shorter than the 60 minute timeout of the job
// in the reusable workflow.
const timeout = 50 * time.Minute

// notStartedJobStatuses are the CircleCI statuses of a job that hasn't
// started.
var notStartedJobStatuses = []string{"blocked", "queued", "not_running", "on_hold"}

// terminalWorkflowStatuses are the CircleCI statuses of a finished workflow.
// CircleCI only re-runs finished workflows.
var terminalWorkflowStatuses = []string{"success", "failed", "error", "canceled", "unauthorized"}

// decision is what to do about the E2E job after looking at it.
type decision int

// Values for decision.
const (
	// decisionStop means there's nothing to re-run.
	decisionStop decision = iota
	// decisionWait means to look at the job again later.
	decisionWait
	// decisionRerun means to re-run the job.
	decisionRerun
)

func main() {
	exitCode := 1
	defer func() {
		os.Exit(exitCode)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	token := strings.TrimSpace(os.Getenv("CIRCLECI_TOKEN"))
	if token == "" {
		actions.Errorf("CIRCLECI_TOKEN is empty")
		return
	}
	jobName := os.Getenv("E2E_JOB_NAME")
	if jobName == "" {
		actions.Errorf("E2E_JOB_NAME is empty")
		return
	}

	ghContext, err := actions.Context()
	if err != nil {
		actions.Errorf("unable to get action context: %v", err)
		return
	}

	client := &circleciClient{baseURL: "https://circleci.com", token: token, http: &http.Client{Timeout: time.Minute}}
	if err := RunAction(ctx, client, ghContext, jobName, pollInterval); err != nil {
		actions.Errorf("%s", err.Error())
		return
	}
	exitCode = 0
}

// RunAction re-runs the CircleCI job named jobName for the pull request's
// head commit if devbase's draft gate halted it before its tests. It looks
// at the job every pollInterval until there's nothing to wait for.
func RunAction(ctx context.Context, client *circleciClient, actionCtx *actions.GitHubContext, jobName string,
	pollInterval time.Duration) error {
	pr, err := gh.ParsePullRequestPayload(actionCtx.Event)
	if err != nil {
		return fmt.Errorf("parse event payload: %w", err)
	}
	owner, repo := pr.Base.Repo.Owner.Login, pr.Base.Repo.Name

	p, err := findPipeline(ctx, client, owner, repo, pr.Head.Ref, pr.Head.SHA)
	if err != nil {
		return fmt.Errorf("find the CircleCI pipeline for %s: %w", pr.Head.SHA, err)
	}
	if p == nil {
		actions.Infof("No CircleCI pipeline for %s yet. Its %s job reads the pull request's state when it runs.", pr.Head.SHA, jobName)
		return nil
	}

	wf, err := findWorkflow(ctx, client, p.ID, jobName)
	if err != nil {
		return fmt.Errorf("find the %s job in CircleCI pipeline %d: %w", jobName, p.Number, err)
	}
	if wf == nil {
		actions.Infof("No workflow in CircleCI pipeline %d has a %s job.", p.Number, jobName)
		return nil
	}
	actions.Infof("Found %s in workflow %s (%s) of CircleCI pipeline %d.", jobName, wf.Name, wf.ID, p.Number)

	for {
		done, err := checkJob(ctx, client, owner, repo, wf.ID, jobName)
		if err != nil || done {
			return err
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for workflow %s: %w", wf.ID, ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// findPipeline returns the newest pipeline for a commit among the first page
// of pipelines on a branch, or nil if there's none.
func findPipeline(ctx context.Context, client *circleciClient, owner, repo, branch, sha string) (*pipeline, error) {
	pipelines, err := client.pipelines(ctx, owner, repo, branch)
	if err != nil {
		return nil, err
	}

	var newest *pipeline
	for i := range pipelines {
		p := &pipelines[i]
		if p.VCS.Revision == sha && (newest == nil || p.CreatedAt.After(newest.CreatedAt)) {
			newest = p
		}
	}
	return newest, nil
}

// findWorkflow returns the newest workflow in a pipeline with a job named
// jobName, or nil if there's none.
func findWorkflow(ctx context.Context, client *circleciClient, pipelineID, jobName string) (*workflow, error) {
	workflows, err := client.workflows(ctx, pipelineID)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(workflows, func(a, b workflow) int { return b.CreatedAt.Compare(a.CreatedAt) })
	for i := range workflows {
		j, err := findJob(ctx, client, workflows[i].ID, jobName)
		if err != nil {
			return nil, err
		}
		if j != nil {
			return &workflows[i], nil
		}
	}
	return nil, nil
}

// findJob returns the job named jobName in a workflow, or nil if there's
// none.
func findJob(ctx context.Context, client *circleciClient, workflowID, jobName string) (*job, error) {
	jobs, err := client.jobs(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	i := slices.IndexFunc(jobs, func(j job) bool { return j.Name == jobName })
	if i < 0 {
		return nil, nil
	}
	return &jobs[i], nil
}

// checkJob looks at the job named jobName in a workflow once, and re-runs it
// if decide says to. It returns true when there's nothing left to wait for.
func checkJob(ctx context.Context, client *circleciClient, owner, repo, workflowID, jobName string) (bool, error) {
	j, err := findJob(ctx, client, workflowID, jobName)
	if err != nil {
		return false, fmt.Errorf("find the %s job in workflow %s: %w", jobName, workflowID, err)
	}
	if j == nil {
		return false, fmt.Errorf("workflow %s has no %s job", workflowID, jobName)
	}

	ranTests := false
	if j.Number != 0 {
		steps, err := client.stepNames(ctx, owner, repo, j.Number)
		if err != nil {
			return false, fmt.Errorf("get the steps of job %d: %w", j.Number, err)
		}
		ranTests = slices.Contains(steps, runTestsStep)
	}

	status, err := client.workflowStatus(ctx, workflowID)
	if err != nil {
		return false, fmt.Errorf("get the status of workflow %s: %w", workflowID, err)
	}

	d, reason := decide(j.Status, ranTests, status)
	switch d {
	case decisionStop:
		actions.Infof("Not re-running %s: %s.", jobName, reason)
		return true, nil
	case decisionRerun:
		newWorkflowID, err := client.rerunJob(ctx, workflowID, j.ID)
		if err != nil {
			return false, fmt.Errorf("re-run job %d in workflow %s: %w", j.Number, workflowID, err)
		}
		actions.Infof("Re-running %s, since %s: https://app.circleci.com/pipelines/workflows/%s", jobName, reason, newWorkflowID)
		return true, nil
	case decisionWait:
		actions.Infof("Waiting, since %s.", reason)
	}
	return false, nil
}

// decide returns what to do about the E2E job, and why, given its status,
// whether it started runTestsStep, and its workflow's status.
func decide(jobStatus string, ranTests bool, workflowStatus string) (d decision, reason string) {
	switch {
	case slices.Contains(notStartedJobStatuses, jobStatus):
		return decisionStop, fmt.Sprintf("the job's status is %s, and the job reads the pull request's state when it starts", jobStatus)
	case ranTests:
		return decisionStop, fmt.Sprintf("the job reached its %q step", runTestsStep)
	case jobStatus == "running":
		return decisionWait, fmt.Sprintf("the job is running and hasn't reached its %q step", runTestsStep)
	case jobStatus != "success":
		return decisionStop, fmt.Sprintf("the job's status is %s, and it didn't reach its %q step", jobStatus, runTestsStep)
	case !slices.Contains(terminalWorkflowStatuses, workflowStatus):
		return decisionWait, fmt.Sprintf("the job was halted before its tests, and its workflow's status is %s", workflowStatus)
	default:
		return decisionRerun, fmt.Sprintf("the job was halted before its tests, and its workflow's status is %s", workflowStatus)
	}
}
