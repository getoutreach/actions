#!/usr/bin/env bash

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

if [[ -z $APP_VERSION ]]; then
  echo "APP_VERSION must be passed to script." >&2
  exit 1
fi

if [[ ! -f "actions.yaml" ]]; then
  echo "Script must be ran in root of repository (actions.yaml needs to exist)." >&2
  exit 1
fi

# This is just to authenticate gcloud in CI, but we can't use the $CI variable because
# it's unset by the releasing script.
if [[ -z $GOOGLE_SERVICE_ACCOUNT ]]; then
  # You have to write this to a file to authenticate gcloud service accounts, can't
  # read it from stdin like you can with docker auth (it is deleted right after auth).
  echo "$GCLOUD_SERVICE_ACCOUNT" >gcloud-auth-key.json

  gcloud auth activate-service-account \
    circleci-rw@outreach-docker.iam.gserviceaccount.com \
    --key-file=gcloud-auth-key.json

  rm -f gcloud-auth-key.json
fi

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
  mapfile -t actions_to_build <<<"$(yq -rc '.actions[]' actions.yaml)"
fi

if [[ ${#actions_to_build[@]} -eq 0 ]]; then
  if [[ $APP_VERSION != "development" ]]; then
    echo "No actions were detected to be built, but we're on the main branch so this is a problem." >&2
    exit 1
  fi

  echo "No actions to build, skipping."
  exit 0
fi

default_build_args=(
  --platform "linux/amd64"
  --ssh default
  --push
)

yq -rc '.actions[]' actions.yaml | while read -r action; do
  for action_to_build in "${actions_to_build[@]}"; do
    if [ "$action_to_build" == "$action" ]; then
      # Action actually exists in yaml list of created actions.

      if [[ $APP_VERSION == "development" ]]; then
        # Before we push another development tag for each action, we should delete the old one if it exists.
        if [[ $(gcloud container images list-tags gcr.io/outreach-docker/actions/"$action" | grep -c development) -gt 0 ]]; then
          # If we're in this conditional it means a development image already exists, but we can't just blindly
          # delete it before making sure it doesn't have any other tags attached to it.
          if [[ $(gcloud container images list-tags gcr.io/outreach-docker/actions/"action" | grep development | awk '{print $2}' | awk -F , '{ for (i = 1; i <= NF; i++) print $i }' | wc -l) -eq 1 ]]; then
            # If we're in this conditional it means that the development image only had the development tag on it, so
            # we're safe to delete it.
            echo " -> Found old development image for $action@$APP_VERSION, deleting before pushing new one"
            gcloud container images delete --force-delete-tags --quiet gcr.io/outreach-docker/actions/"$action":development
          fi
        fi
      fi

      echo " -> Building and pushing docker image for $action@$APP_VERSION"

      build_args=("${default_build_args[@]}")
      build_args+=(
        --build-arg ACTION="$action"
        -t "gcr.io/outreach-docker/actions/$action:$APP_VERSION"
      )

      if [[ $APP_VERSION != "development" ]]; then
        # If we're building images from the "release" branch, tag all images with latest.
        build_args+=(
          -t "gcr.io/outreach-docker/actions/$action:latest"
        )
      fi

      docker buildx build "${build_args[@]}" .
    fi
  done
done
