#!/usr/bin/env bash

set -eo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

if [[ ! -d actions ]]; then
  echo "Script needs to be ran from the root of the repository." >&2
  exit 1
fi

if [[ $# -ne 1 ]]; then
  echo "Nameof action must be passed to script (if you're using make do \"make new-action name=<name>\")." >&2
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

"$DIR"/shell-wrapper.sh yq.sh '.actions[]' actions.yaml | while read -r action; do
  if [[ $action == "$newAction" ]]; then
    echo "Action named \"$action\" already exists." >&2
    exit 1
  fi
done

echo " -> Creating action \"$newAction\" boilerplate"

mkdir -p "actions/$newAction"

cat <<EOF >.github/workflows/"$newAction".yaml
# yaml-language-server: \$schema=https://json.schemastore.org/github-workflow
name: "$newAction"
on:
  workflow_call:
    secrets:
      PAT_OUTREACH_CI:
        required: false
    inputs:
      image_tag:
        type: string
        description: "Release to run the action from: latest, or a version such as 1.2.3"
        default: latest
        required: false

jobs:
  run:
    # ubuntu-slim has no Docker daemon, so run the release binary.
    runs-on: ubuntu-slim
    steps:
      - name: Download action
        uses: getoutreach/actions/.github/actions/download@main
        with:
          name: "$newAction"
          version: \${{ inputs.image_tag }}
      - name: Run action
        working-directory: \${{ runner.temp }}
        env:
          GITHUB_TOKEN: \${{ secrets.GITHUB_TOKEN }}
          PAT_OUTREACH_CI: \${{ secrets.PAT_OUTREACH_CI }}
        run: ./$newAction

EOF

cat <<EOF >test/"$newAction".yaml
# yaml-language-server: \$schema=https://json.schemastore.org/github-workflow
name: $newAction local test
on: pull_request

jobs:
  run:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/getoutreach/action-$newAction:local
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

	"github.com/getoutreach/actions/pkg/gh"
	"github.com/google/go-github/v75/github"
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
		actions.Errorf("%s", err.Error())
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

./scripts/shell-wrapper.sh yq.sh --yaml-output '.actions += ["'"$newAction"'"]' actions.yaml >actions.yaml.new
mv actions.yaml.new actions.yaml

# Add a release build, so the workflow can download the action's binary.
# Inserted as text before `archives:` to keep the file's comments and anchors.
build="$(
  cat <<EOF
  - main: ./actions/$newAction
    id: &$newAction $newAction
    binary: *$newAction
    goos:
      - linux
    goarch:
      - amd64
    ldflags:
      - "-w -s"
    env:
      - CGO_ENABLED=0
EOF
)"
# Blank lines are held back, so the build lands right after the last one.
BUILD="$build" awk '
  /^$/ { blank++; next }
  /^archives:/ { print ENVIRON["BUILD"] }
  { for (; blank > 0; blank--) print ""; print }
' .goreleaser.yml >.goreleaser.yml.new
mv .goreleaser.yml.new .goreleaser.yml
make fmt
