#!/usr/bin/env bash

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

if [[ ! -d actions ]]; then
  echo "Script needs to be ran from the root of the repository." >&2
  exit 1
fi

if [[ $# -lt 1 ]]; then
  echo "Script expects at least one argument passed to it, the name of the action to run (if you're using make do \"make test-action name=<name>\")." >&2
  exit 1
fi

action="$1"
shift

if [[ $# -eq 1 ]]; then
  payload="test/payloads/$1.json"
  echo " -> Custom payload \"$payload\" requested"
fi

if [[ "$("$DIR"/shell-wrapper.sh yq.sh --raw-output --compact-output "any(.actions[] == \"$action\"; .)" actions.yaml)" == "false" ]]; then
  echo "Action set to be tested (\"$action\") does not exist." >&2
  exit 1
fi

image_url="ghcr.io/getoutreach/action-$action"

echo " -> Building local docker image ($image_url:local)"

docker buildx build --platform "linux/amd64" \
  --ssh default -t "$image_url:local" \
  --build-arg ACTION="$action" --load .

act_args=(
  --container-architecture linux/amd64
  --secret GITHUB_TOKEN="$(cat "$HOME"/.outreach/github.token)"
)

testWorkflow="test/$action.yaml"
if [[ ! -f $testWorkflow ]]; then
  echo "Could not find test workflow file at \"$testWorkflow\"." >&2
  exit 1
fi

act_args+=(
  --workflows "$testWorkflow"
)

on="$(grep on: <"$testWorkflow" | head -1 | awk '{print $2}' | xargs)"
if [[ -z $on ]]; then
  echo 'Could not parse action event trigger from "on" key in test workflow file.' >&2
  exit 1
fi

echo " -> Found event trigger \"$on\" in \"$testWorkflow\""

if [[ -z $payload ]]; then
  payload="test/payloads/$on.json"
fi

if [[ ! -f $payload ]]; then
  echo " -> No payload exists for the detected event trigger (looked for \"$payload\")."
else
  act_args+=(
    --eventpath "$payload"
  )
fi

echo " -> Using payload \"$payload\" for detected event trigger"
echo " -> Running local test for \"$action\""

mise exec -- act "${act_args[@]}" "$on"
