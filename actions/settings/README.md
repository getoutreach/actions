# settings

The `settings` GitHub Action is meant to enforce a common set of repository settings at an organization
level (achieved by placing in action in a `.github` repository for an organization, as opposed to individual
repository's `.github` folders). This consistency is achieved by running this action on four primary events
for each repository under an organization:

* [Repository (created)](https://docs.github.com/en/developers/webhooks-and-events/webhooks/webhook-events-and-payloads#repository)
* [Repository (edited)](https://docs.github.com/en/developers/webhooks-and-events/webhooks/webhook-events-and-payloads#repository)
* [Branch Protection Rule (created)](https://docs.github.com/en/developers/webhooks-and-events/webhooks/webhook-events-and-payloads#branch_protection_rule)
* [Branch Protection Rule (edited)](https://docs.github.com/en/developers/webhooks-and-events/webhooks/webhook-events-and-payloads#branch_protection_rule)

The primary issue with this action is that only branch protection rules are available as an event that
will trigger a workflow ([ref](https://docs.github.com/en/actions/using-workflows/events-that-trigger-workflows#branch_protection_rule)).
So we're left with one other option to actually get accurate invocations of this action and that is the
[repository_dispatch](https://docs.github.com/en/actions/using-workflows/events-that-trigger-workflows#repository_dispatch) event.

The `repository_dispatch` event basically allows you to utilize the GitHub API to trigger a webhook event
that falls underneath that category (`repository_dispatch`) of event for the action. You can pass data to
the action through the payload of that API call. The idea would be to set up a small service that listens
for webhook events on all of the four primary events referenced above, as they do all generate webhook
events, and send `repository_dispatch` API calls against the `.github` repository that this action is
embedded in. The payload will contain repository owner and name, and the default branch as well as all other
necessary information to run the action can be inferred from that.
