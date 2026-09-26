# Sandboxes (Greenhouse)

A sandbox is an isolated cloud development environment: a persistent workspace
you open in the browser with a file editor, a terminal, a git panel, and an
agent. They are for development, not for serving traffic.

## Isolation

Sandboxes run under the `kata` RuntimeClass (a micro-VM boundary) with a
NetworkPolicy that allows **internet egress only**: they can reach out but not
into your cluster. Owners and admins see every sandbox in the organization;
everyone else sees their own. Boot progress is reported stage by stage.

## Git

Status, branches, diff, log, checkout/create, commit all or named files,
uncommit (soft reset), fetch with prune, pull (optionally rebasing), push, force
push with `--force-with-lease`, and opening a pull request with `gh`.

## The agent

With an Anthropic API key configured (**Instance admin → Settings**), a sandbox
runs Claude against its workspace with tools to list, read and write files and
run commands. Conversations are kept per sandbox, can be pointed at a repository
directory, renamed and deleted.

## Deleting

Deleting a sandbox deletes its workspace volume. Push first.
