#!/usr/bin/env bash

set -eo pipefail

# It's important to note that all of this script relies on heavy assumptions on how
# it's called. Right now that currently means it's called in CI by either the
# shell/ci/release/dryrun.sh or shell/ci/release/release.sh scripts in devbase and
# ran by semantic-release via yarn within those scripts.
#
# Those scripts unset all environment variables set by CircleCI and mimics what would
# happen if the branch was already squashed and merged onto main in the case of the
# dry-run. So we can't really use any branch comparison and/or conditionals based off
# of the current branch. We instead opt to assume APP_VERSION will be set to
# "development" if running in dry-run mode and we take advantage of the fact that the
# entirety of the current branch is squashed into the HEAD commit.

REPO_SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DEVBASE_LIB_DIR="$REPO_SCRIPTS_DIR/../.bootstrap/shell/lib"

# shellcheck source=../.bootstrap/shell/lib/box.sh
source "$DEVBASE_LIB_DIR"/box.sh

# shellcheck source=../.bootstrap/shell/lib/docker.sh
source "$DEVBASE_LIB_DIR"/docker.sh

# shellcheck source=../.bootstrap/shell/lib/logging.sh
source "$DEVBASE_LIB_DIR"/logging.sh

if [[ -z $APP_VERSION ]]; then
  fatal "APP_VERSION must be passed to script."
fi

if [[ ! -f "actions.yaml" ]]; then
  fatal "Script must be ran in root of repository (actions.yaml needs to exist)."
fi

# Wrapper around gojq to make it easier to use with yaml files.
yamlq() {
  "$REPO_SCRIPTS_DIR"/shell-wrapper.sh yq.sh --raw-output --compact-output "$@"
}

GITHUB_ORG="${GITHUB_ORG:-$(get_box_field org)}"

actions_to_build=()
if [[ $APP_VERSION == "development" ]]; then
  # Figure out which actions have changed.
  if [[ "$(git diff --name-only HEAD^ HEAD | grep -c actions/)" -gt 0 ]]; then
    # The reason we do the line number check first is because mapfile leaves us with an array with
    # a length of 1 even if nothing is there for some reason.
    mapfile -t actions_to_build <<<"$(git diff --name-only HEAD^ HEAD | grep actions/ | awk -F / '{print $2}')"
  fi
else
  # Build and push all docker images for each action.
  mapfile -t actions_to_build <<<"$(yamlq '.actions[]' actions.yaml)"
fi

if [[ ${#actions_to_build[@]} -eq 0 ]]; then
  if [[ $APP_VERSION != "development" ]]; then
    fatal "No actions were detected to be built, but we're on the main branch so this is a problem."
  fi

  info "No actions to build, skipping."
  exit 0
fi

default_build_args=(
  --platform "linux/amd64"
  --ssh default
  --push
)

yamlq '.actions[]' actions.yaml | while read -r action; do
  ghcr_image_name="action-$action"
  ghcr_image_url="ghcr.io/$GITHUB_ORG/$ghcr_image_name"
  push_registries="$(get_docker_push_registries)"
  for action_to_build in "${actions_to_build[@]}"; do
    if [ "$action_to_build" == "$action" ]; then
      # Action actually exists in yaml list of created actions.
      info_sub "Building and pushing Docker image for $action@$APP_VERSION"

      build_args=("${default_build_args[@]}")
      build_args+=(
        --build-arg ACTION="$action"
        --tag "$ghcr_image_url:$APP_VERSION"
      )

      for push_registry in $push_registries; do
        build_args+=(
          --tag "$push_registry/actions/$action:$APP_VERSION"
        )
      done

      if [[ $APP_VERSION != "development" ]]; then
        # If we're building images from the "release" branch, tag all images with latest.
        build_args+=(
          --tag "$ghcr_image_url:latest"
        )
        for push_registry in $push_registries; do
          build_args+=(
            --tag "$push_registry/actions/$action:latest"
          )
        done
      fi

      docker buildx build "${build_args[@]}" .
    fi
  done
done
