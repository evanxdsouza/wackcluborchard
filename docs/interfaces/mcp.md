# MCP

Wack Club Orchard speaks the Model Context Protocol over streamable HTTP at `/mcp` (or
`https://mcp.<domain>/mcp` when the installer publishes the MCP host). Agents
authenticate with a personal API token and can do what that person can do.

```bash
claude mcp add --transport http wackcluborchard https://wackcluborchard.example.com/mcp \
  --header "Authorization: Bearer wackclubwackcluborchard_…"
```

## Tools

| Tool | Does |
| --- | --- |
| `list_organizations`, `list_projects` | What exists |
| `get_app`, `list_deploys`, `get_logs` | Status, history, recent logs and crash reports |
| `create_app`, `deploy_app`, `restart_app`, `scale_app`, `rollback_app` | Change apps |
| `set_variables` | Project or app variables; affected apps roll out |
| `create_database`, `query_database` | PostgreSQL |
| `list_jobs`, `run_job`, `get_job_run` | Jobs with per-step output |
| `list_templates`, `deploy_template` | The Grove |

Deploys are asynchronous: poll `get_app` or `list_deploys`. Instance admins can
turn the endpoint off under **Instance admin → Settings**.
