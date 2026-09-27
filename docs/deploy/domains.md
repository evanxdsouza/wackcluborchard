# Domains and HTTPS

Every app with an HTTP port gets `<name>.<app domain>` through one shared
Gateway with a wildcard certificate. If the name is taken, it becomes
`<name>-<project>`.

## Custom domains

**App → Domains → Add.** Point a `CNAME` (or `A` record) at the instance, then
add the hostname. Wack Club Orchard creates an Ingress annotated for cert-manager, which
issues a certificate over HTTP-01 once the record resolves.

## Auth wall

**App → Settings → Auth wall** puts the app behind Wack Club Orchard sign-in: only members
of the project get through. It uses Traefik's forwardAuth; the dashboard issues a
short-lived ticket, the app's host trades it for a 12-hour signed cookie, and the
app receives `X-Wackclubwackcluborchard-User` and `X-Wackclubwackcluborchard-Email` headers.

## Raw TCP and UDP

Non-HTTP ports are reachable inside the project at `<app>:<port>`. Mark a port
**public** to publish it on the organization's shared IP as a NodePort.
