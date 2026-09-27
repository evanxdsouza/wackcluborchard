# HTTP API

Everything the dashboard does goes through `/api`, JSON in and out. Authenticate
with a session cookie or `Authorization: Bearer wackclubwackcluborchard_…` (create tokens under
**Account → API tokens**). Cookie-authenticated writes must come from the
dashboard's origin.

Some useful endpoints:

| | |
| --- | --- |
| `GET /api/orgs/{org}/overview` | Projects with apps, databases and latest metrics |
| `POST /api/projects/{id}/apps` | Create an app (`{"name","source":{"type":"image","image":…},"ports","deploy":true}`) |
| `POST /api/apps/{id}/deploy` | Build or roll out; returns the deploy and a build ticket |
| `PATCH /api/apps/{id}` | Replicas, resources, ports, volumes, health, auth wall |
| `POST /api/apps/{id}/rollback/{deploy}` | Roll back |
| `GET /api/apps/{id}/logs?pod=&since=15m&previous=1` | Server-sent log lines |
| `GET /api/deploys/{id}/logs` | Server-sent build log |
| `PUT /api/projects/{id}/variables` | Upsert variables, or import `{"env": ".env text"}` |
| `PUT /api/projects/{id}/compose` | Apply a compose file (`dryRun` for a plan) |
| `POST /api/databases/{id}/query` | Run SQL |
| `POST /api/jobs/{id}/runs` | Trigger a job |
| `GET /api/events?topics=app:{id},project:{id}` | Server-sent state changes; reconnects replay from `Last-Event-ID` |
| `GET /api/apps/{id}/shell` | WebSocket: stdin lines in, output out |
