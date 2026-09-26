package api

import (
	"net/http"

	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

func (s *Server) routes() {
	m := s.mux
	h := func(pattern string, f http.HandlerFunc) { m.HandleFunc(pattern, f) }

	h("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })

	// auth
	h("GET /api/auth/state", s.authState)
	h("POST /api/auth/signup", s.signup)
	h("POST /api/auth/login", s.login)
	h("POST /api/auth/logout", s.logout)
	h("POST /api/auth/claim", s.claim)
	h("POST /api/auth/passkey/login/begin", s.passkeyLoginBegin)
	h("POST /api/auth/passkey/login/finish", s.passkeyLoginFinish)
	h("POST /api/auth/passkey/register/begin", s.authed(s.passkeyRegisterBegin))
	h("POST /api/auth/passkey/register/finish", s.authed(s.passkeyRegisterFinish))
	h("GET /api/auth/sso/start", s.ssoStart)
	h("GET /api/auth/sso/callback", s.ssoCallback)
	h("GET /api/auth/forward", s.forwardAuth)
	h("GET /api/auth/wall", s.wall)

	// account
	h("GET /api/me", s.authed(s.me))
	h("PATCH /api/me", s.authed(s.updateMe))
	h("POST /api/me/password", s.authed(s.changePassword))
	h("GET /api/me/tokens", s.authed(s.listTokens))
	h("POST /api/me/tokens", s.authed(s.createToken))
	h("DELETE /api/me/tokens/{id}", s.authed(s.deleteToken))
	h("POST /api/me/ssh-keys", s.authed(s.addSSHKey))
	h("DELETE /api/me/ssh-keys/{id}", s.authed(s.deleteSSHKey))
	h("DELETE /api/me/passkeys/{id}", s.authed(s.deletePasskey))

	// orgs
	h("POST /api/orgs", s.authed(s.createOrg))
	h("GET /api/orgs/{org}", s.authed(s.getOrg))
	h("PATCH /api/orgs/{org}", s.authed(s.updateOrg))
	h("DELETE /api/orgs/{org}", s.authed(s.deleteOrg))
	h("GET /api/orgs/{org}/overview", s.authed(s.overview))
	h("GET /api/orgs/{org}/deploys/recent", s.authed(s.recentDeploys))
	h("GET /api/orgs/{org}/members", s.authed(s.listMembers))
	h("POST /api/orgs/{org}/members", s.authed(s.invite))
	h("PATCH /api/orgs/{org}/members/{user}", s.authed(s.updateMember))
	h("DELETE /api/orgs/{org}/members/{user}", s.authed(s.removeMember))
	h("DELETE /api/orgs/{org}/invites/{id}", s.authed(s.deleteInvite))
	h("GET /api/orgs/{org}/audit", s.authed(s.audit))
	h("GET /api/orgs/{org}/usage", s.authed(s.usage))
	h("POST /api/orgs/{org}/scim-token", s.authed(s.rotateSCIM))
	h("GET /api/orgs/{org}/jobs", s.authed(s.listJobs))
	h("GET /api/orgs/{org}/sandboxes", s.authed(s.listSandboxes))
	h("POST /api/orgs/{org}/sandboxes", s.authed(s.createSandbox))
	h("POST /api/orgs/{org}/projects", s.authed(s.createProject))
	h("GET /api/invites/{token}", s.getInvite)
	h("POST /api/invites/{token}/accept", s.authed(s.acceptInvite))
	h("GET /api/search", s.authed(s.search))
	h("GET /api/templates", s.authed(s.listTemplates))
	h("GET /api/events", s.authed(s.events))

	// projects
	h("GET /api/projects/{id}", s.authed(s.getProject))
	h("PATCH /api/projects/{id}", s.authed(s.updateProject))
	h("DELETE /api/projects/{id}", s.authed(s.deleteProject))
	h("POST /api/projects/{id}/environments", s.authed(s.addEnvironment))
	h("DELETE /api/projects/{id}/environments/{env}", s.authed(s.deleteEnvironment))
	h("POST /api/projects/{id}/members", s.authed(s.addProjectMember))
	h("DELETE /api/projects/{id}/members/{user}", s.authed(s.removeProjectMember))
	h("GET /api/projects/{id}/variables", s.authed(s.listVariables))
	h("PUT /api/projects/{id}/variables", s.authed(s.setVariables))
	h("GET /api/projects/{id}/compose", s.authed(s.getCompose))
	h("PUT /api/projects/{id}/compose", s.authed(s.applyCompose))
	h("POST /api/projects/{id}/apps", s.authed(s.createApp))
	h("POST /api/projects/{id}/databases", s.authed(s.createDatabase))
	h("POST /api/projects/{id}/jobs", s.authed(s.createJob))
	h("POST /api/projects/{id}/templates", s.authed(s.createTemplateInstance))
	h("DELETE /api/template-instances/{id}", s.authed(s.deleteTemplateInstance))

	// apps
	h("GET /api/apps/{id}", s.authed(s.getApp))
	h("PATCH /api/apps/{id}", s.authed(s.updateApp))
	h("DELETE /api/apps/{id}", s.authed(s.deleteApp))
	h("POST /api/apps/{id}/deploy", s.authed(s.deployApp))
	h("POST /api/apps/{id}/restart", s.authed(s.restartApp))
	h("POST /api/apps/{id}/rollback/{dep}", s.authed(s.rollbackApp))
	h("GET /api/apps/{id}/deploys", s.authed(s.listDeploys))
	h("GET /api/apps/{id}/logs", s.authed(s.appLogs))
	h("GET /api/apps/{id}/metrics", s.authed(s.appMetrics))
	h("GET /api/apps/{id}/events", s.authed(s.appEvents))
	h("GET /api/apps/{id}/crashes", s.authed(s.appCrashes))
	h("GET /api/apps/{id}/shell", s.authed(s.appShell))
	h("POST /api/apps/{id}/domains", s.authed(s.addDomain))
	h("DELETE /api/apps/{id}/domains/{host}", s.authed(s.removeDomain))
	h("GET /api/deploys/{id}", s.authed(s.getDeploy))
	h("GET /api/deploys/{id}/logs", s.authed(s.deployLogs))

	// databases
	h("GET /api/databases/{id}", s.authed(s.getDatabase))
	h("PATCH /api/databases/{id}", s.authed(s.updateDatabase))
	h("DELETE /api/databases/{id}", s.authed(s.deleteDatabase))
	h("POST /api/databases/{id}/restart", s.authed(s.restartDatabase))
	h("POST /api/databases/{id}/query", s.authed(s.queryDatabase))
	h("POST /api/databases/{id}/backups", s.authed(s.backupDatabase))
	h("GET /api/databases/{id}/crashes", s.authed(s.dbCrashes))
	h("GET /api/databases/{id}/terminal", s.authed(s.dbTerminal))

	// jobs
	h("GET /api/jobs/{id}", s.authed(s.getJob))
	h("PUT /api/jobs/{id}", s.authed(s.updateJob))
	h("DELETE /api/jobs/{id}", s.authed(s.deleteJob))
	h("POST /api/jobs/{id}/runs", s.authed(s.runJob))
	h("GET /api/runs/{id}", s.authed(s.getRun))
	h("POST /api/runs/{id}/cancel", s.authed(s.cancelRun))

	// sandboxes
	h("GET /api/sandboxes/{id}", s.authed(s.getSandbox))
	h("DELETE /api/sandboxes/{id}", s.authed(s.deleteSandbox))
	h("POST /api/sandboxes/{id}/restart", s.authed(s.restartSandbox))
	h("GET /api/sandboxes/{id}/files", s.authed(s.sandboxFiles))
	h("GET /api/sandboxes/{id}/file", s.authed(s.sandboxReadFile))
	h("PUT /api/sandboxes/{id}/file", s.authed(s.sandboxWriteFile))
	h("POST /api/sandboxes/{id}/git", s.authed(s.sandboxGit))
	h("GET /api/sandboxes/{id}/shell", s.authed(s.sandboxShell))
	h("POST /api/sandboxes/{id}/conversations", s.authed(s.createConversation))
	h("PATCH /api/sandboxes/{id}/conversations/{conv}", s.authed(s.updateConversation))
	h("DELETE /api/sandboxes/{id}/conversations/{conv}", s.authed(s.updateConversation))
	h("POST /api/sandboxes/{id}/conversations/{conv}/messages", s.authed(s.sendMessage))

	// github
	h("GET /api/github/status", s.authed(s.githubStatus))
	h("POST /api/github/manifest", s.superadmin(s.githubManifest))
	h("GET /api/github/manifest/callback", s.superadmin(s.githubManifestCallback))
	h("GET /api/github/connect", s.authed(s.githubConnect))
	h("GET /api/github/callback", s.authed(s.githubCallback))
	h("GET /api/github/setup", s.authed(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		http.Redirect(w, r, "/account", http.StatusFound)
	}))
	h("GET /api/github/repos", s.authed(s.githubRepos))
	h("GET /api/github/branches", s.authed(s.githubBranches))
	h("GET /api/github/inspect", s.authed(s.githubInspect))
	h("POST /api/github/webhook", s.githubWebhook)

	// instance admin
	h("GET /api/admin/settings", s.superadmin(s.adminSettings))
	h("PATCH /api/admin/settings", s.superadmin(s.updateAdminSettings))
	h("GET /api/admin/orgs", s.superadmin(s.adminOrgs))
	h("GET /api/admin/users", s.superadmin(s.adminUsers))
	h("PATCH /api/admin/users/{id}", s.superadmin(s.adminUpdateUser))
	h("GET /api/admin/nodes", s.superadmin(s.adminNodes))
	h("POST /api/admin/pools", s.superadmin(s.adminSavePool))
	h("DELETE /api/admin/pools/{id}", s.superadmin(s.adminDeletePool))
	h("GET /api/admin/networking", s.superadmin(s.adminNetworking))
	h("GET /api/admin/audit", s.superadmin(s.adminAudit))

	// SCIM
	h("GET /scim/v2/Users", s.scimUsers)
	h("POST /scim/v2/Users", s.scimCreateUser)
	h("GET /scim/v2/Users/{id}", s.scimUser)
	h("PATCH /scim/v2/Users/{id}", s.scimUser)
	h("PUT /scim/v2/Users/{id}", s.scimUser)
	h("DELETE /scim/v2/Users/{id}", s.scimUser)
	h("GET /scim/v2/ServiceProviderConfig", func(w http.ResponseWriter, r *http.Request) {
		scimJSON(w, 200, map[string]any{"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"}, "patch": map[string]bool{"supported": true}, "bulk": map[string]bool{"supported": false}, "filter": map[string]any{"supported": true, "maxResults": 1000}})
	})

	// MCP
	h("POST /mcp", s.mcp)
	h("GET /mcp", s.mcp)
	h("DELETE /mcp", s.mcp)

	// frontend
	h("GET /", s.static)
}
