# The wackcluborchard CLI

```bash
wackcluborchard login https://wackcluborchard.example.com   # paste a token, or sign in with a password
wackcluborchard apps
wackcluborchard status api
wackcluborchard deploy api                  # follows the build log
wackcluborchard deploy api --image ghcr.io/me/api:1.2
wackcluborchard logs api -f --since 15m
wackcluborchard logs api --previous          # the container that died
wackcluborchard scale api 3
wackcluborchard restart api
wackcluborchard rollback api                 # previous successful image; or: wackcluborchard rollback api 12
wackcluborchard env Homelab                  # list
wackcluborchard env Homelab KEY=value --app api --secret
wackcluborchard db query postgres "select count(*) from users"
wackcluborchard jobs
wackcluborchard run nightly-report           # streams step output, exits non-zero on failure
wackcluborchard open api
wackcluborchard mcp                          # prints the command to add Wack Club Orchard to Claude Code
```

Apps can be named `project/app` when names repeat. In CI, set `WACKCLUBORCHARD_URL` and
`WACKCLUBORCHARD_TOKEN` instead of logging in.

## wackcluborchardctl (on the box)

```bash
sudo wackcluborchardctl status
sudo wackcluborchardctl claim [--url host]
sudo wackcluborchardctl public-ip set 203.0.113.10 --apply
sudo wackcluborchardctl hostname add mango --apply
sudo wackcluborchardctl mcp enable --domain mcp.example.com --apply
sudo wackcluborchardctl signup invite
sudo wackcluborchardctl config set ingress.host wackcluborchard.example.com
sudo wackcluborchardctl update                # helm upgrade with /etc/wackcluborchard/values.yaml
sudo wackcluborchardctl logs -f
```
