// Copyright 2022 Outreach Corporation. All Rights Reserved.

package main

import (
	"fmt"
	"testing"

	"github.com/google/go-github/v43/github"
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
