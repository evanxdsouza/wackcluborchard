# Jobs and pipelines

Jobs run work that is not a long-lived server: cleanups, imports, migrations,
reports. They run on the cluster, not on anyone's laptop.

A job has a **name**, an optional **schedule**, a **concurrency policy**, and
one or more **steps**:

| Step | Runs |
| --- | --- |
| Script | `bash`, `python` or `node` you type in |
| App command | A command in one of your apps' current image, with its variables |
| Custom image | Any image with your own command |

Steps run in order as init containers of one pod, so each has its own logs and
exit code, a failing step stops the pipeline, and later steps show as skipped.

## Scheduling

Standard five-field cron in UTC, or `@hourly`/`@daily`/`@weekly`/`@monthly`.
Without a schedule a job runs when triggered: dashboard, `orchard run`, or the
`run_job` MCP tool. The concurrency policy decides what happens when the
previous run is still going: **skip**, **queue** or **allow**.

> A step that ends `|| true` always reports success. The editor warns about it.

## The image matters

Script steps run in small stock images: `debian:12-slim` (no `curl`, no `wget`),
`python:3.13-slim`, `node:22-slim`. Install what you need on the first line, or
use a custom image.
