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

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DEVBASE_LIB_DIR="$DIR/../.bootstrap/shell/lib"

# shellcheck source=../.bootstrap/shell/lib/box.sh
source "$DEVBASE_LIB_DIR"/box.sh

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
  "$DIR"/shell-wrapper.sh yq.sh --raw-output --compact-output "$@"
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
  image_name="action-$action"
  image_url="ghcr.io/$GITHUB_ORG/$image_name"
  for action_to_build in "${actions_to_build[@]}"; do
    if [ "$action_to_build" == "$action" ]; then
      # Action actually exists in yaml list of created actions.

      if [[ $APP_VERSION == "development" ]]; then
        # Before we push another development tag for each action, we should delete any other images with that tag.
        dev_version_ids="$(gh api /orgs/$GITHUB_ORG/packages/container/$image_name/versions --jq '.[] | select(.metadata.container.tags | any(. == "development")) | .id')"
        if [[ $dev_version_ids =~ ^[[:digit:][:space:]]+$ ]]; then
          info_sub "Deleting old development images for $image_name: $dev_version_ids"
          for version_id in $dev_version_ids; do
            gh api -X DELETE "/orgs/$GITHUB_ORG/packages/container/$image_name/versions/$version_id"
          done
        fi
      fi

      info_sub "Building and pushing Docker image for $action@$APP_VERSION"

      build_args=("${default_build_args[@]}")
      build_args+=(
        --build-arg ACTION="$action"
        -t "$image_url:$APP_VERSION"
      )

      if [[ $APP_VERSION != "development" ]]; then
        # If we're building images from the "release" branch, tag all images with latest.
        build_args+=(
          -t "$image_url:latest"
        )
      fi

      docker buildx build "${build_args[@]}" .
    fi
  done
done
