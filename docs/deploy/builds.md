# Builds

Builds run as Jobs on your cluster. Each one clones the repo in an init
container, builds with rootless BuildKit (`buildctl-daemonless.sh`), and pushes
to the configured registry with a registry-backed layer cache per app.

## The five steps

1. **Scheduling builder**: waiting for a node with capacity
2. **Building image**: BuildKit, with streaming logs
3. **Pushing image**: to the registry
4. **Creating app**: server-side apply of the Kubernetes objects, then waiting for health checks
5. **Done**

Build status comes from the Job's own state, so a builder that dies is reported
as failed instead of hanging in "building" forever. A control-plane restart marks
in-flight builds failed with a message saying so.

## Concurrency

Builds run in a bounded number of slots (**Instance admin → Settings → Build
slots**, sized by the installer from RAM). A queued build is **accepted, not
ignored**: it starts on its own when a slot frees, and the API says so:

```json
{ "build": { "accepted": true, "started": false, "status": "queued", "running": 2, "slots": 2, "waiting": 1,
  "message": "All 2 build slots are busy on this instance. …" } }
```

Do not retry: a second request for the same app supersedes the waiting one
rather than moving it up the queue.

## Build arguments and pull credentials

Variables prefixed `BUILD_` become `--build-arg` values (without the prefix).
For registries that reject anonymous pulls in `FROM`, set on the app as secrets:

- `DHI_USERNAME` + `DHI_TOKEN` for Docker Hardened Images (`dhi.io`)
- or `DOCKER_AUTH_CONFIG`, a Docker config JSON whose `auths` map lists each registry

They are used only for pulls during the build.

## Isolation

Without Kata, builds run rootless but with unconfined seccomp/AppArmor, the same
posture as Docker-based platforms: fine when the person building owns the box.
For other people's code, install Kata and turn on **Kata VMs for image builds**.
