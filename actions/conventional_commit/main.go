// Copyright 2022 Outreach Corporation. All Rights Reserved.

package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/getoutreach/actions/pkg/gh"
	"github.com/google/go-github/v72/github"
	"github.com/pkg/errors"
	actions "github.com/sethvargo/go-githubactions"
)

// allowedCommitTypes are the allowed strings in the type named capture group in the
// reConventionalCommit regular expression.
var allowedCommitTypes = map[string]struct{}{
	"feat":     {},
	"fix":      {},
	"docs":     {},
	"style":    {},
	"refactor": {},
	"perf":     {},
	"test":     {},
	"build":    {},
	"ci":       {},
	"chore":    {},
	"revert":   {},
}

// bypassAuthorEmails are the authors that are allowed to bypass the conventional commit
// check. This is read from the environment variable "BYPASS_AUTHOR_EMAILS", as well as
// the defaults list below.
//
// Note: Commits must have a valid GPG signature to bypass the check.
var bypassAuthorEmails = map[string]struct{}{
	// Static email for dependabot app.
	"49699333+dependabot[bot]@users.noreply.github.com": {},
}

// Variable block for regular expression parsing.
var (
	// reConventionalCommit is a regular expression that matches a valid conventional
	// commit title.
	//
	// For examples, see https://regex101.com/r/gkNDNK/1
	reConventionalCommit = regexp.MustCompile(`^(?P<type>\w+)(?P<scope>\([-\w\/]+\))?(?P<breaking>!)?:\s(?P<message>.*?)$`)

	// reConventionalCommitType stores the index of the type named capture group for
	// reConventionalCommit.
	reConventionalCommitType = reConventionalCommit.SubexpIndex("type")

	// reConventionalCommitScope stores the index of the scope named capture group for
	// reConventionalCommit.
	reConventionalCommitScope = reConventionalCommit.SubexpIndex("scope")

	// reConventionalCommitBreaking stores the index of the breaking named capture
	// group for reConventionalCommit.
	reConventionalCommitBreaking = reConventionalCommit.SubexpIndex("breaking")

	// reConventionalCommitMessage stores the index of the message named capture
	// group for reConventionalCommit.
	reConventionalCommitMessage = reConventionalCommit.SubexpIndex("message")
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
		actions.Errorf("%s", err.Error())
		return
	}
	exitCode = 0
}

// allowBypass checks if the commit author is allowed to bypass the conventional commit
// check.
func allowBypass(commit *github.RepositoryCommit) bool {
	// Read emails from BYPASS_AUTHOR_EMAILS env var in bypassAuthorEmails.
	//
	// Note: This parsing is here to make it easy to unit test.
	if bypassEmails := os.Getenv("BYPASS_AUTHOR_EMAILS"); bypassEmails != "" {
		for _, email := range strings.Split(os.Getenv("BYPASS_AUTHOR_EMAILS"), " ") {
			bypassAuthorEmails[email] = struct{}{}
		}
	}

	// check if the commit is by an author that is allowed to bypass the check.
	authorEmail := commit.GetCommit().GetAuthor().GetEmail()
	if _, ok := bypassAuthorEmails[authorEmail]; ok {
		actions.Infof("author %q is allowed to bypass conventional commit check", authorEmail)

		// to ensure that someone doesn't try to bypass this check by spoofing the email
		// address, we check that the commit has a valid GPG signature (according to Github).
		if !commit.GetCommit().GetVerification().GetVerified() {
			actions.Errorf("commit %q is not signed, not bypassing check", commit.GetSHA())
		} else {
			// the commit is signed and from an approved email, so we can bypass the check.
			return true
		}
	}

	// always default to not allowing bypass.
	return false
}

// validateCommitMessage checks if the commit message meets conventional commit requirement or not.
func validateCommitMessage(commitMessage string) error {
	matches := reConventionalCommit.FindStringSubmatch(commitMessage)
	if matches == nil {
		return errors.New("pr title does not match conventional commit syntax")
	}

	cType := matches[reConventionalCommitType]
	scope := matches[reConventionalCommitScope]
	breaking := matches[reConventionalCommitBreaking]
	message := matches[reConventionalCommitMessage]

	if _, exists := allowedCommitTypes[cType]; !exists {
		return fmt.Errorf("commit type %q is not in the list of allowed commit types", cType)
	}

	actions.Infof("successfully parsed conventional commit:\ntype: [%s]\nscope: [%s]\nbreaking: [%t]\nmessage: [%s]",
		cType, strings.TrimSuffix(strings.TrimPrefix(scope, "("), ")"), breaking == "!", message)

	return nil
}

// RunAction is where the actual implementation of the GitHub action goes and is called
// by func main.
func RunAction(ctx context.Context, client *github.Client, actionCtx *actions.GitHubContext) error {
	if actionCtx.EventName != "pull_request" {
		return errors.New("conventional_commit running on non \"pull_request\" event")
	}

	pr, err := gh.ParsePullRequestPayload(actionCtx.Event)
	if err != nil {
		return errors.Wrap(err, "parse event payload")
	}

	actions.Infof("PR title (sans quotes): %q", pr.Title)
	actions.Infof("number of commits: %d", pr.Commits)

	if pr.Commits == 1 {
		// The title of the first commit and the PR title need to match in this case.
		commit, _, err := client.Repositories.GetCommit(ctx, pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pr.Head.SHA, &github.ListOptions{})
		if err != nil {
			return errors.Wrap(err, "get first commit details from github api")
		}

		// check if the commit author is allowed to bypass the conventional commit check.
		if allowBypass(commit) {
			return nil
		}

		message := commit.GetCommit().GetMessage()
		parsedMessage := strings.Split(strings.ReplaceAll(message, "\r\n", "\n"), "\n")
		commitTitle := parsedMessage[0]

		actions.Infof("parsed title of first commit (sans quotes): %q", commitTitle)

		if strings.TrimSpace(commitTitle) != strings.TrimSpace(pr.Title) {
			return errors.New("since branch has 1 commit, PR title and commit title must match and both be in conventional commit format")
		}
	}

	return validateCommitMessage(pr.Title)
}
