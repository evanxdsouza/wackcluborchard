# Logs and shell

## Live logs

An app's **Overview** streams logs as they happen over server-sent events.
Filter by **pod**, by **time range**, or with the text filter. The skull button
reads the **previous** container: the one that died.

## Crash reports

When a container is killed, Orchard captures its last 300 lines before
Kubernetes throws them away, OOM kills included, under **Observe → Crash
reports**. Databases get the same under **Terminal → Killed**.

| Symptom | Usually |
| --- | --- |
| Exits immediately, module or import error | Missing dependency or the wrong build stage |
| Exits after a few seconds, connection refused | It cannot reach its database |
| Killed, exit 137 | Out of memory |
| Runs but never becomes ready | Health check path or port is wrong |

## Shell

**Shell** runs a shell in a running container over a WebSocket to the pod's exec
endpoint, with the same authentication as the dashboard. It is a real process in
a real pod; changes are lost on restart. Opening one is recorded in the audit log.

## Kubernetes events

**Observe → Events** lists warnings recorded for the app's pods, newest first,
with repeat counts and a plain-English translation next to the raw reason. The
alert banner above the tabs shows only what is wrong *now*.
