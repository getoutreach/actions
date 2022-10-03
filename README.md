# actions
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/getoutreach/actions)
[![Generated via Bootstrap](https://img.shields.io/badge/Outreach-Bootstrap-%235951ff)](https://github.com/getoutreach/bootstrap)
[![Coverage Status](https://coveralls.io/repos/github/getoutreach/actions/badge.svg?branch=main)](https://coveralls.io/github//getoutreach/actions?branch=main)
<!-- <<Stencil::Block(extraBadges)>> -->

<!-- <</Stencil::Block>> -->

This is a collection of actions that are used by Outreach.


## Contributing

Please read the [CONTRIBUTING.md](CONTRIBUTING.md) document for guidelines on developing and contributing changes.

## High-level Overview

<!-- <<Stencil::Block(overview)>> -->

This repository houses importable GitHub Actions that can be used in other repositories.
These actions are written in Go and executed via a Dockerfile within the workflow itself.

### Creating a new Action

Boilerplate code can be generated for a new action by running the following command:

```bash
make new-action name=<name_of_action>
```

The name of the action cannot contain uppercase letters or spaces. Running the command
does a few things:

- Adds an entry to the `actions` list in `actions.yaml`.
- Creates a reusable (from other repositories) workflow file in `.github/workflows/`
- Creates a test workflow file in `test/`
- Creates a directory to define the action with boilerplate go code in `actions/`

### Testing an Action

You can test your action using the `act` CLI (`nektos/act`) with the following command:

```bash
make test-action name=<name_of_action>
```

Optionally you can explicitly provide a payload too:

```bash
make test-action name=<name_of_action> payload=<payload_name>
```

The payloads are found in `test/payloads` and if you explicitly provide one you should only
provide the basename of the payload file without the extension
(`test/payloads/pull_request.json` would be referred to by `pull_request`). If you do not
explicitly provide a payload it will automatically try to use the a payload by looking at
the value of the `on` key in the action's YAML file found in `test/`.

If you have an action with a complex `on` key (multiple triggers or triggers with filters)
then you'll have to pass payload explicitly as the script will not be able to automatically
parse it.

### Using Actions in Other Repositories

To use actions created in this repository in other repositories you can follow this
"Shared Workflows" pattern for various types of event triggers (`on: ...`):

```yaml
name: Pull Request Shared Actions
on: pull_request

jobs:
  conventional_commit:
    name: Conventional Commit
    uses: getoutreach/actions/.github/workflows/conventional_commit.yaml@main
    secrets:
      OUTREACH_DOCKER_JSON: ${{ secrets.OUTREACH_DOCKER_JSON }}
```

This file would be placed in `.github/workflows/pull-request-shared-actions.yaml`. To
add more shared workflows triggered by pull requests to this file, you'd just add a new
key under `jobs` that looks similar to the one already there, changing the key, name,
and uses path accordingly.

### Configuring an Action to Make Org-Wide GitHub API Requests

By default, all actions can make GitHub API requests that are local to the repository
that it is running in. If an action needs the ability to make org-wide GitHub API
requests you'll need to add a key-value pair to the
`jobs[].<job-id>.secrets.GITHUB_TOKEN` key:
`PAT_OUTREACH_CI: ${{ secrets.PAT_OUTREACH_CI }}`. As well as set the `orgWideAccess`
flag in the parameters of `gh.NewClient` to true in the Go code that defines the action.

<!-- <</Stencil::Block>> -->
