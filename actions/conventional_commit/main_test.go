// Copyright 2022 Outreach Corporation. All Rights Reserved.

package main

import (
	"fmt"
	"testing"

	"github.com/google/go-github/v43/github"
	"gotest.tools/v3/assert"
)

func Test_allowBypass(t *testing.T) {
	type args struct {
		commit *github.RepositoryCommit
	}
	tests := []struct {
		name                  string
		bypassAuthorEmailsEnv string
		args                  args
		want                  bool
	}{
		{
			name: "should allow bypass for entry in bypassAuthorEmails",
			args: args{
				commit: &github.RepositoryCommit{
					Commit: &github.Commit{
						Author: &github.CommitAuthor{
							Email: github.String("49699333+dependabot[bot]@users.noreply.github.com"),
						},
						Verification: &github.SignatureVerification{
							Verified: github.Bool(true),
						},
					},
				},
			},
			want: true,
		},
		{
			name: "should not allow bypass for entry not in bypassAuthorEmails",
			args: args{
				commit: &github.RepositoryCommit{
					Commit: &github.Commit{
						Author: &github.CommitAuthor{
							Email: github.String("jaredallard@users.noreply.github.com"),
						},
						Verification: &github.SignatureVerification{
							Verified: github.Bool(true),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "should not allow bypass for entry in bypassAuthorEmails with unverified commit",
			args: args{
				commit: &github.RepositoryCommit{
					Commit: &github.Commit{
						SHA: github.String("1234"),
						Author: &github.CommitAuthor{
							Email: github.String("49699333+dependabot[bot]@users.noreply.github.com"),
						},
						Verification: &github.SignatureVerification{
							Verified: github.Bool(false),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "should read bypass author emails from env",
			args: args{
				commit: &github.RepositoryCommit{
					Commit: &github.Commit{
						Author: &github.CommitAuthor{
							Email: github.String("jaredallard@users.noreply.github.com"),
						},
						Verification: &github.SignatureVerification{
							Verified: github.Bool(true),
						},
					},
				},
			},
			bypassAuthorEmailsEnv: "jaredallard@users.noreply.github.com",
			want:                  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.bypassAuthorEmailsEnv != "" {
				fmt.Printf("BYPASS_AUTHOR_EMAILS=%q\n", tt.bypassAuthorEmailsEnv)
				t.Setenv("BYPASS_AUTHOR_EMAILS", tt.bypassAuthorEmailsEnv)
			}

			if got := allowBypass(tt.args.commit); got != tt.want {
				t.Errorf("allowBypass() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_validateCommitMessage(t *testing.T) {
	type args struct {
		commitMessage string
	}
	tests := []struct {
		name   string
		args   args
		errMsg string
	}{
		{
			name: "fix",
			args: args{
				commitMessage: "fix(pencil): stop graphite breaking when too much pressure applied",
			},
			errMsg: "",
		},
		{
			name: "feat",
			args: args{
				commitMessage: "feat(pencil): add 'graphiteWidth' option",
			},
			errMsg: "",
		},
		{
			name: "feat without space",
			args: args{
				commitMessage: "feat(pencil):add 'graphiteWidth' option",
			},
			errMsg: "pr title does not match conventional commit syntax",
		},
		{
			name: "invalid type",
			args: args{
				commitMessage: "invalid(pencil): add 'graphiteWidth' option",
			},
			errMsg: "commit type \"invalid\" is not in the list of allowed commit types",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCommitMessage(tt.args.commitMessage)
			if tt.errMsg != "" {
				assert.Error(t, err, tt.errMsg)
			} else {
				assert.NilError(t, err)
			}
		})
	}
}
