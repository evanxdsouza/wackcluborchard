# Going to production

- **Claim the instance and add a passkey** (or a password) immediately.
- **Decide signups**: `open`, `invite` or `closed` (Instance admin → Settings, or `wackcluborchardctl signup`).
- **HTTPS everywhere**: set `FRONTEND_URL` to the https URL so cookies are Secure and passkeys work.
- **Back up `/data`**: the state volume holds users, projects and secrets. Snapshot it, or copy `wackcluborchard.json` (it is written atomically).
- **Database backups off the box**: logical backups live on the database volume; ship them elsewhere for disaster recovery.
- **Isolation** if you run other people's code: gVisor for tenants, Kata for builds and sandboxes.
- **Quotas**: set organization caps and member allowances before inviting people.
- **DNS**: check **Instance admin → Networking** shows every name resolving to the public IP.
- **Set a default public IP** for TCP/UDP ports and public databases: `sudo wackcluborchardctl public-ip set <ip> --apply`.
