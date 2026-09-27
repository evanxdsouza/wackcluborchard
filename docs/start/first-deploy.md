# First deploy

The shortest path: a repo with a Dockerfile, deployed, reachable, with a database.

```
GitHub repo ─push─▶ Build on cluster ─▶ Push image ─▶ Roll out + health checks ─▶ HTTPS URL
                                                         └─ every image kept ─▶ Rollback
```

## 1. Connect GitHub

An instance admin creates the GitHub App from **Instance admin → GitHub App**
with the **manifest flow**: Wack Club Orchard generates the manifest, GitHub creates the
app, and the credentials come back automatically. Then each person links their
own account under **Account → GitHub**; Wack Club Orchard only sees the repositories that
account granted.

## 2. Create an app

**New → GitHub repository.** Pick the repo and a branch. Wack Club Orchard reads the
repository and fills in the **Dockerfile** and its **build stages** (it targets
the last stage), the **port** from `EXPOSE`, and the **variables** declared with
`ENV`/`ARG`.

Check the build stage: a multi-stage Dockerfile built at `base` produces an image
with no application in it.

## 3. Deploy

You land on a progress page with five steps (Scheduling builder, Building image,
Pushing image, Creating app, Done) and the build log streaming as it happens. A
failed build leaves the previous version running. Every push to the branch
deploys itself from then on.

## 4. Give it a database

**New → PostgreSQL database.** When it is ready the Connection tab shows URI,
psql, .env and JDBC forms with copy buttons. Instead of pasting the password,
reference it from the app's variables:

```
DATABASE_URL=${{ postgres.DATABASE_URL }}
```

## 5. Watch it run

The app's **Overview** has live logs on the left and CPU, memory and replicas on
the right. The replica slider scales it. **Shell** opens a terminal inside a
running container.

## If the build fails

- **Wrong build stage.** See above.
- **The app needs a database at build time.** Migrations belong at start, not build.
- **The repo does not build with its own Dockerfile.** Wack Club Orchard runs exactly what
  the Dockerfile says; it usually fails locally too.

A crash loop after a successful build shows as **Failed** with the dead
container's own logs under **Observe → Crash reports**.
