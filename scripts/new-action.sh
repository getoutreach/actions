#!/usr/bin/env bash

if [[ ! -d actions ]]; then
  echo "Script needs to be ran from the root of the repository." >&2
  exit 1
fi

if [[ $# -ne 1 ]]; then
  echo "Name of action must be passed to script (if you're using make do \"make new-action name=<name>\")." >&2
  exit 1
fi
newAction="$1"

if [[ $newAction =~ [[:upper:]] ]]; then
  # this is a docker image tag name restriction
  echo "Action name cannot contain uppercase characters." >&2
  exit 1
fi

if [[ $newAction =~ [[:space:]] ]]; then
  # this is a docker image tag name restriction AND github action restriction
  echo "Action name cannot contain spaces." >&2
  exit 1
fi

yq -rc '.actions[]' actions.yaml | while read -r action; do
  if [[ $action == "$newAction" ]]; then
    echo "Action named \"$action\" already exists." >&2
    exit 1
  fi
done

echo " -> Creating action \"$newAction\" boilerplate"

mkdir -p "actions/$newAction"

cat <<EOF >.github/workflows/"$newAction".yaml
# yaml-language-server: \$schema=https://json.schemastore.org/github-workflow
name: $newAction
on:
  workflow_call:
    secrets:
      OUTREACH_DOCKER_JSON:
        required: true
      PAT_OUTREACH_CI:
        required: false
    inputs:
      image_tag:
        type: string
        default: latest
        required: false

jobs:
  run:
    runs-on: ubuntu-latest
    container:
      image: gcr.io/outreach-docker/actions/$newAction:\${{ inputs.image_tag }}
      env:
        GITHUB_TOKEN: \${{ secrets.GITHUB_TOKEN }}
        PAT_OUTREACH_CI: \${{ secrets.PAT_OUTREACH_CI }}
      credentials:
        username: _json_key
        password: \${{ secrets.OUTREACH_DOCKER_JSON }}
    steps:
      - run: /usr/local/bin/action

EOF

cat <<EOF >test/"$newAction".yaml
# yaml-language-server: \$schema=https://json.schemastore.org/github-workflow
name: $newAction local test
on: pull_request

jobs:
  run:
    runs-on: ubuntu-latest
    container:
      image: gcr.io/outreach-docker/actions/$newAction:local
      env:
        GITHUB_TOKEN: \${{ secrets.GITHUB_TOKEN }}
        PAT_OUTREACH_CI: \${{ secrets.GITHUB_TOKEN }}
    steps:
      - run: /usr/local/bin/action

EOF

year="$(date +%Y)"
cat <<EOF >actions/"$newAction"/main.go
// Copyright $year Outreach Corporation. All Rights Reserved.

package main

import (
	"context"
	"os"
	"time"

	"github.com/getoutreach/actions/internal/gh"
	"github.com/google/go-github/v43/github"
	"github.com/pkg/errors"
	actions "github.com/sethvargo/go-githubactions"
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
		actions.Errorf(err.Error())
		return
	}
	exitCode = 0
}

// RunAction is where the actual implementation of the GitHub action goes and is called
// by func main.
func RunAction(ctx context.Context, client *github.Client, actionCtx *actions.GitHubContext) error {
	return errors.New("$newAction is unimplemented")
}

EOF

yq -yi '.actions += ["'"$newAction"'"]' actions.yaml
make fmt
