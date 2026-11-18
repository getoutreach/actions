// Copyright 2026 Outreach Corporation. All Rights Reserved.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	actions "github.com/sethvargo/go-githubactions"
	"gotest.tools/v3/assert"
)

func TestDecide(t *testing.T) {
	tests := []struct {
		name           string
		jobStatus      string
		ranTests       bool
		workflowStatus string
		want           decision
	}{
		{name: "blocked job", jobStatus: "blocked", workflowStatus: "running", want: decisionStop},
		{name: "queued job", jobStatus: "queued", workflowStatus: "running", want: decisionStop},
		{name: "not running job", jobStatus: "not_running", workflowStatus: "running", want: decisionStop},
		{name: "job on hold", jobStatus: "on_hold", workflowStatus: "on_hold", want: decisionStop},
		{name: "job running its tests", jobStatus: "running", ranTests: true, workflowStatus: "running", want: decisionStop},
		{name: "job passed its tests", jobStatus: "success", ranTests: true, workflowStatus: "success", want: decisionStop},
		{name: "job failed its tests", jobStatus: "failed", ranTests: true, workflowStatus: "failed", want: decisionStop},
		{name: "job running before its tests", jobStatus: "running", workflowStatus: "running", want: decisionWait},
		{name: "halted job in a running workflow", jobStatus: "success", workflowStatus: "running", want: decisionWait},
		{name: "halted job in a failing workflow", jobStatus: "success", workflowStatus: "failing", want: decisionWait},
		{name: "halted job in a workflow on hold", jobStatus: "success", workflowStatus: "on_hold", want: decisionWait},
		{name: "halted job in a successful workflow", jobStatus: "success", workflowStatus: "success", want: decisionRerun},
		{name: "halted job in a failed workflow", jobStatus: "success", workflowStatus: "failed", want: decisionRerun},
		{name: "halted job in an errored workflow", jobStatus: "success", workflowStatus: "error", want: decisionRerun},
		{name: "halted job in a canceled workflow", jobStatus: "success", workflowStatus: "canceled", want: decisionRerun},
		{name: "halted job in an unauthorized workflow", jobStatus: "success", workflowStatus: "unauthorized", want: decisionRerun},
		{name: "failed job", jobStatus: "failed", workflowStatus: "failed", want: decisionStop},
		{name: "canceled job", jobStatus: "canceled", workflowStatus: "canceled", want: decisionStop},
		{name: "job not run", jobStatus: "not_run", workflowStatus: "failed", want: decisionStop},
		{name: "job infrastructure failure", jobStatus: "infrastructure_fail", workflowStatus: "failed", want: decisionStop},
		{name: "job timed out", jobStatus: "timedout", workflowStatus: "failed", want: decisionStop},
		{name: "job terminated", jobStatus: "terminated-unknown", workflowStatus: "failed", want: decisionStop},
		{name: "unauthorized job", jobStatus: "unauthorized", workflowStatus: "unauthorized", want: decisionStop},
		{name: "retried job", jobStatus: "retried", workflowStatus: "success", want: decisionStop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := decide(tt.jobStatus, tt.ranTests, tt.workflowStatus)
			assert.Equal(t, got, tt.want, "reason: %s", reason)
		})
	}
}

const (
	testToken = "test-token"
	// testBranch has a + to check that the branch is query-escaped, since an
	// unescaped + decodes to a space.
	testBranch  = "feature/e2e+draft"
	testSHA     = "0123456789abcdef"
	testJobName = "shared/e2e"
)

// haltedSteps are the steps of an E2E job that the draft gate halted.
var haltedSteps = []string{"Spin up environment", "Checkout code", "Skip E2E Tests on Draft PRs"}

// testJob is a job that runs before the E2E job.
var testJob = fakeJob{id: "test-job", name: "shared/test", status: "success", number: 1, steps: []string{"Run tests"}}

// e2eJob returns an E2E job with the given status, job number and steps.
func e2eJob(status string, number int, steps ...string) fakeJob {
	return fakeJob{id: "e2e-job", name: testJobName, status: status, number: number, steps: steps}
}

func TestRunAction(t *testing.T) {
	tests := []struct {
		name       string
		pipelines  []fakePipeline
		wantReruns []rerunRequest
	}{
		{
			name: "re-runs a halted job in a finished workflow",
			pipelines: []fakePipeline{
				{id: "other-commit", revision: "fedcba9876543210", createdAt: "2026-11-16T10:00:00.000Z"},
				{id: "older-pipeline", revision: testSHA, createdAt: "2026-11-16T08:00:00.000Z", workflows: []*fakeWorkflow{
					{id: "older-workflow", createdAt: "2026-11-16T08:00:01.000Z", statuses: []string{"success"},
						jobs: []fakeJob{testJob, e2eJob("success", 2, "Spin up environment", runTestsStep)}},
				}},
				{id: "pipeline", revision: testSHA, createdAt: "2026-11-16T09:00:00.000Z", workflows: []*fakeWorkflow{
					{id: "workflow", createdAt: "2026-11-16T09:00:01.000Z", statuses: []string{"success"},
						jobs: []fakeJob{testJob, e2eJob("success", 3, haltedSteps...)}},
				}},
			},
			wantReruns: []rerunRequest{
				{WorkflowID: "workflow", Body: map[string]any{"jobs": []any{"e2e-job"}, "sparse_tree": true}},
			},
		},
		{
			name: "skips a job that ran its tests",
			pipelines: []fakePipeline{
				{id: "pipeline", revision: testSHA, createdAt: "2026-11-16T09:00:00.000Z", workflows: []*fakeWorkflow{
					{id: "workflow", createdAt: "2026-11-16T09:00:01.000Z", statuses: []string{"success"},
						jobs: []fakeJob{testJob, e2eJob("success", 2, "Spin up environment", "Skip E2E Tests on Draft PRs", runTestsStep)}},
				}},
			},
		},
		{
			name: "skips a job that hasn't started",
			pipelines: []fakePipeline{
				{id: "pipeline", revision: testSHA, createdAt: "2026-11-16T09:00:00.000Z", workflows: []*fakeWorkflow{
					{id: "workflow", createdAt: "2026-11-16T09:00:01.000Z", statuses: []string{"running"},
						jobs: []fakeJob{testJob, e2eJob("blocked", 0)}},
				}},
			},
		},
		{
			name: "skips a commit without a pipeline",
			pipelines: []fakePipeline{
				{id: "other-commit", revision: "fedcba9876543210", createdAt: "2026-11-16T09:00:00.000Z", workflows: []*fakeWorkflow{
					{id: "workflow", createdAt: "2026-11-16T09:00:01.000Z", statuses: []string{"success"},
						jobs: []fakeJob{testJob, e2eJob("success", 2, haltedSteps...)}},
				}},
			},
		},
		{
			name: "skips a pipeline without the job",
			pipelines: []fakePipeline{
				{id: "pipeline", revision: testSHA, createdAt: "2026-11-16T09:00:00.000Z", workflows: []*fakeWorkflow{
					{id: "workflow", createdAt: "2026-11-16T09:00:01.000Z", statuses: []string{"success"}, jobs: []fakeJob{testJob}},
				}},
			},
		},
		{
			name: "looks at the newest workflow with the job",
			pipelines: []fakePipeline{
				{id: "pipeline", revision: testSHA, createdAt: "2026-11-16T09:00:00.000Z", workflows: []*fakeWorkflow{
					{id: "workflow", createdAt: "2026-11-16T09:00:01.000Z", statuses: []string{"success"},
						jobs: []fakeJob{testJob, e2eJob("success", 2, haltedSteps...)}},
					{id: "unrelated-workflow", createdAt: "2026-11-16T09:30:00.000Z", statuses: []string{"success"},
						jobs: []fakeJob{{id: "other-job", name: "other", status: "success", number: 4}}},
					{id: "rerun-workflow", createdAt: "2026-11-16T09:20:00.000Z", statuses: []string{"running"},
						jobs: []fakeJob{{id: "rerun-e2e-job", name: testJobName, status: "queued", number: 3}}},
				}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeCircleCI{pipelines: tt.pipelines}
			client := f.start(t)

			err := RunAction(testRunContext(t), client, testGitHubContext(), testJobName, time.Millisecond)
			assert.NilError(t, err)
			f.mu.Lock()
			defer f.mu.Unlock()
			assert.DeepEqual(t, f.reruns, tt.wantReruns)
		})
	}
}

func TestRunActionWaitsForTheWorkflowToFinish(t *testing.T) {
	f := &fakeCircleCI{pipelines: []fakePipeline{
		{id: "pipeline", revision: testSHA, createdAt: "2026-11-16T09:00:00.000Z", workflows: []*fakeWorkflow{
			{id: "workflow", createdAt: "2026-11-16T09:00:01.000Z", statuses: []string{"running", "failing", "failed"},
				jobs: []fakeJob{testJob, e2eJob("success", 2, haltedSteps...)}},
		}},
	}}
	client := f.start(t)

	err := RunAction(testRunContext(t), client, testGitHubContext(), testJobName, time.Millisecond)
	assert.NilError(t, err)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, f.statusLookups["workflow"], 3)
	assert.DeepEqual(t, f.reruns, []rerunRequest{
		{WorkflowID: "workflow", Body: map[string]any{"jobs": []any{"e2e-job"}, "sparse_tree": true}},
	})
}

func TestRunActionReturnsCircleCIErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "Project not found"})
	}))
	t.Cleanup(srv.Close)
	client := &circleciClient{baseURL: srv.URL, token: testToken, http: srv.Client()}

	err := RunAction(testRunContext(t), client, testGitHubContext(), testJobName, time.Millisecond)
	assert.ErrorContains(t, err, "404 Not Found: Project not found")
}

// testRunContext returns a context that stops a RunAction that never
// returns.
func testRunContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// testGitHubContext returns the context of a pull_request event for
// testBranch at testSHA.
func testGitHubContext() *actions.GitHubContext {
	return &actions.GitHubContext{
		EventName: "pull_request",
		Event: map[string]any{
			"action": "ready_for_review",
			"pull_request": map[string]any{
				"number": 7,
				"draft":  false,
				"head":   map[string]any{"ref": testBranch, "sha": testSHA},
				"base": map[string]any{"repo": map[string]any{
					"name":  "example",
					"owner": map[string]any{"login": "getoutreach"},
				}},
			},
		},
	}
}

// fakeJob is a job in a fakeWorkflow.
type fakeJob struct {
	id, name, status string
	// number is the job number, or 0 for none, like a job that hasn't
	// started.
	number int
	// steps are the names of the steps the job started.
	steps []string
}

// fakeWorkflow is a workflow in a fakePipeline.
type fakeWorkflow struct {
	id, createdAt string
	// statuses are what successive workflow lookups return. The last one
	// repeats.
	statuses []string
	jobs     []fakeJob
}

// fakePipeline is a pipeline on testBranch.
type fakePipeline struct {
	id, revision, createdAt string
	workflows               []*fakeWorkflow
}

// rerunRequest is a workflow re-run that fakeCircleCI received.
type rerunRequest struct {
	WorkflowID string
	Body       map[string]any
}

// fakeCircleCI serves the CircleCI API endpoints that RunAction calls for
// the getoutreach/example project. Its list endpoints return one item per
// page, so every test exercises pagination.
type fakeCircleCI struct {
	pipelines []fakePipeline

	mu sync.Mutex
	// statusLookups counts the lookups of each workflow, by ID.
	statusLookups map[string]int
	reruns        []rerunRequest
}

// start serves f until the test ends and returns a client for it.
func (f *fakeCircleCI) start(t *testing.T) *circleciClient {
	f.statusLookups = map[string]int{}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v2/project/gh/getoutreach/example/pipeline", f.listPipelines)
	mux.HandleFunc("GET /api/v2/pipeline/{id}/workflow", f.listWorkflows)
	mux.HandleFunc("GET /api/v2/workflow/{id}/job", f.listJobs)
	mux.HandleFunc("GET /api/v2/workflow/{id}", f.getWorkflow)
	mux.HandleFunc("POST /api/v2/workflow/{id}/rerun", f.rerunWorkflow)
	mux.HandleFunc("GET /api/v1.1/project/github/getoutreach/example/{number}", f.getJob)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Circle-Token") != testToken {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"message": "You must log in first."})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	return &circleciClient{baseURL: srv.URL, token: testToken, http: srv.Client()}
}

func (f *fakeCircleCI) listPipelines(w http.ResponseWriter, r *http.Request) {
	items := []map[string]any{}
	if r.URL.Query().Get("branch") == testBranch {
		for i, p := range f.pipelines {
			items = append(items, map[string]any{
				"id":         p.id,
				"number":     i + 1,
				"state":      "created",
				"created_at": p.createdAt,
				"vcs":        map[string]any{"branch": testBranch, "revision": p.revision},
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_page_token": nil})
}

func (f *fakeCircleCI) listWorkflows(w http.ResponseWriter, r *http.Request) {
	for _, p := range f.pipelines {
		if p.id != r.PathValue("id") {
			continue
		}
		items := []map[string]any{}
		for _, wf := range p.workflows {
			items = append(items, map[string]any{
				"id":          wf.id,
				"name":        "release",
				"pipeline_id": p.id,
				"status":      wf.statuses[0],
				"created_at":  wf.createdAt,
			})
		}
		writePage(w, r, items)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"message": "Pipeline not found"})
}

func (f *fakeCircleCI) listJobs(w http.ResponseWriter, r *http.Request) {
	wf := f.workflow(r.PathValue("id"))
	if wf == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "Workflow not found"})
		return
	}
	items := []map[string]any{}
	for _, j := range wf.jobs {
		item := map[string]any{"id": j.id, "name": j.name, "status": j.status, "type": "build"}
		if j.number != 0 {
			item["job_number"] = j.number
		}
		items = append(items, item)
	}
	writePage(w, r, items)
}

func (f *fakeCircleCI) getWorkflow(w http.ResponseWriter, r *http.Request) {
	wf := f.workflow(r.PathValue("id"))
	if wf == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "Workflow not found"})
		return
	}
	status := wf.statuses[min(f.statusLookups[wf.id], len(wf.statuses)-1)]
	f.statusLookups[wf.id]++
	writeJSON(w, http.StatusOK, map[string]any{"id": wf.id, "name": "release", "status": status, "created_at": wf.createdAt})
}

func (f *fakeCircleCI) rerunWorkflow(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": err.Error()})
		return
	}
	f.reruns = append(f.reruns, rerunRequest{WorkflowID: r.PathValue("id"), Body: body})
	writeJSON(w, http.StatusAccepted, map[string]any{"workflow_id": "new-" + r.PathValue("id")})
}

func (f *fakeCircleCI) getJob(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": err.Error()})
		return
	}
	for _, p := range f.pipelines {
		for _, wf := range p.workflows {
			for _, j := range wf.jobs {
				if j.number != number {
					continue
				}
				steps := []map[string]any{}
				for _, s := range j.steps {
					steps = append(steps, map[string]any{"name": s, "actions": []any{}})
				}
				writeJSON(w, http.StatusOK, map[string]any{"build_num": number, "steps": steps})
				return
			}
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"message": "Build not found"})
}

// workflow returns the workflow with the given ID, or nil.
func (f *fakeCircleCI) workflow(id string) *fakeWorkflow {
	for _, p := range f.pipelines {
		for _, wf := range p.workflows {
			if wf.id == id {
				return wf
			}
		}
	}
	return nil
}

// writePage writes the page of items that the request's page-token asks for,
// with one item per page.
func writePage(w http.ResponseWriter, r *http.Request, items []map[string]any) {
	i := 0
	if token := r.URL.Query().Get("page-token"); token != "" {
		i, _ = strconv.Atoi(token)
	}
	page := map[string]any{"items": items[min(i, len(items)):min(i+1, len(items))], "next_page_token": nil}
	if i+1 < len(items) {
		page["next_page_token"] = strconv.Itoa(i + 1)
	}
	writeJSON(w, http.StatusOK, page)
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
