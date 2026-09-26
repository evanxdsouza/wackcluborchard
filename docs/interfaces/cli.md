# The orchard CLI

```bash
orchard login https://orchard.example.com   # paste a token, or sign in with a password
orchard apps
orchard status api
orchard deploy api                  # follows the build log
orchard deploy api --image ghcr.io/me/api:1.2
orchard logs api -f --since 15m
orchard logs api --previous          # the container that died
orchard scale api 3
orchard restart api
orchard rollback api                 # previous successful image; or: orchard rollback api 12
orchard env Homelab                  # list
orchard env Homelab KEY=value --app api --secret
orchard db query postgres "select count(*) from users"
orchard jobs
orchard run nightly-report           # streams step output, exits non-zero on failure
orchard open api
orchard mcp                          # prints the command to add Orchard to Claude Code
```

Apps can be named `project/app` when names repeat. In CI, set `ORCHARD_URL` and
`ORCHARD_TOKEN` instead of logging in.

## orchardctl (on the box)

```bash
sudo orchardctl status
sudo orchardctl claim [--url host]
sudo orchardctl public-ip set 203.0.113.10 --apply
sudo orchardctl hostname add mango --apply
sudo orchardctl mcp enable --domain mcp.example.com --apply
sudo orchardctl signup invite
sudo orchardctl config set ingress.host orchard.example.com
sudo orchardctl update                # helm upgrade with /etc/orchard/values.yaml
sudo orchardctl logs -f
```
