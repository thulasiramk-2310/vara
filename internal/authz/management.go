package authz

// Management capabilities (VARA-RFC-0019 §7.1). RFC-0019 extends RFC-0018's
// closed vocabulary with the operations of the repository control plane. Each is
// operation-centric — it exists only because it maps to one observable
// control-plane request — exactly as the data-plane capabilities do.
//
// Scope note: create-repo and list-repos are authorized against the SERVER
// resource (ServerResource, below); the rest are authorized against a repository,
// like the RFC-0018 capabilities.
const (
	CapCreateRepo Capability = "create-repo" // server: POST /_vara/repositories
	CapListRepos  Capability = "list-repos"  // server: GET  /_vara/repositories
	CapDeleteRepo Capability = "delete-repo" // repo:   DELETE /_vara/repositories/{repo}
	CapRenameRepo Capability = "rename-repo" // repo:   POST /_vara/repositories/{repo}/rename (old name)
	CapAdmin      Capability = "admin"       // repo:   read/modify policy & metadata
	CapArchive    Capability = "archive"     // repo:   archive/unarchive — reserved (§5.4)

	// CapManageAccounts is server-scoped (RFC-0020 §8.3): it authorizes account
	// administration (create/disable/delete/set-password for any account). An
	// account changing its OWN password does not require it.
	CapManageAccounts Capability = "manage-accounts"
)

// ServerResource is the reserved policy key naming the host itself (RFC-0019
// §7.2, resource id "*"). Server-scoped capabilities (create-repo, list-repos)
// are authorized against it, and it maps to <policy-root>/_server.json. Because
// repository names may not begin with "_" (RFC-0019 §10), no repository can
// collide with this key.
const ServerResource = "_server"

// Register the RFC-0019 capabilities into the closed set so a policy file naming
// them validates (RFC-0018 §7.1 fail-fast). Kept in this file so the RFC-0019
// additions live beside their definitions rather than editing RFC-0018's set.
func init() {
	for _, c := range []Capability{
		CapCreateRepo, CapListRepos, CapDeleteRepo, CapRenameRepo, CapAdmin, CapArchive,
		CapManageAccounts,
	} {
		Known[c] = true
	}
}
