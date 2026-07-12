package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/identity"
	"github.com/thulasiramk-2310/vara/internal/protocol"
	"github.com/thulasiramk-2310/vara/internal/repomanager"
)

// This file is the RFC-0019 control plane: the JSON management API under
// /_vara/repositories. It reuses the server's authentication preamble (authn)
// and authorization enforcer verbatim — one authn path, one authz path, two
// route families — and delegates all lifecycle to the Repository Manager, which
// alone orchestrates content/policy/metadata. The handlers here only translate
// HTTP <-> manager calls and map errors to status codes (RFC-0019 §8).

// handleListRepos: GET /_vara/repositories. Requires `list-repos` on the server.
func (s *Server) handleListRepos(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authn(w, r)
	if !ok {
		return
	}
	if !s.authorizeServer(w, id, authz.CapListRepos) {
		return
	}
	metas, err := s.manager.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, err.Error())
		return
	}
	resp := protocol.ListReposResponse{Repositories: make([]protocol.RepositoryDescriptor, 0, len(metas))}
	for _, m := range metas {
		resp.Repositories = append(resp.Repositories, descriptorOf(m))
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleCreateRepo: POST /_vara/repositories. Requires `create-repo` on the
// server. The creating identity becomes the owner (RFC-0019 §7.3).
func (s *Server) handleCreateRepo(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authn(w, r)
	if !ok {
		return
	}
	if !s.authorizeServer(w, id, authz.CapCreateRepo) {
		return
	}
	var req protocol.CreateRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "decode create request: "+err.Error())
		return
	}
	md, err := s.manager.Create(req.Name, id.ID, repomanager.Visibility(req.Visibility), req.Description)
	if err != nil {
		s.writeManagerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, descriptorOf(md))
}

// handleGetRepo: GET /_vara/repositories/{repo}. Requires `admin` OR `read` on
// the repository (§7.1).
func (s *Server) handleGetRepo(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authn(w, r)
	if !ok {
		return
	}
	repo := r.PathValue("repo")
	if !s.authorizeAny(w, id, repo, authz.CapAdmin, authz.CapRead) {
		return
	}
	md, err := s.manager.Get(repo)
	if err != nil {
		s.writeManagerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, descriptorOf(md))
}

// handleDeleteRepo: DELETE /_vara/repositories/{repo}. Requires `delete-repo`.
func (s *Server) handleDeleteRepo(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authn(w, r)
	if !ok {
		return
	}
	repo := r.PathValue("repo")
	if !s.authorize(w, id, authz.CapDeleteRepo, repo) {
		return
	}
	if err := s.manager.Delete(repo); err != nil {
		s.writeManagerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRenameRepo: POST /_vara/repositories/{repo}/rename. Because a rename
// makes a new name begin to exist, it requires BOTH `rename-repo` on the old
// repository AND `create-repo` on the server (RFC-0019 §5.7, §7.1). Both are
// checked before any effect.
func (s *Server) handleRenameRepo(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authn(w, r)
	if !ok {
		return
	}
	repo := r.PathValue("repo")
	if !s.authorize(w, id, authz.CapRenameRepo, repo) {
		return
	}
	if !s.authorizeServer(w, id, authz.CapCreateRepo) {
		return
	}
	var req protocol.RenameRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "decode rename request: "+err.Error())
		return
	}
	md, err := s.manager.Rename(repo, req.NewName)
	if err != nil {
		s.writeManagerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, descriptorOf(md))
}

// handleReserved answers the routes RFC-0019 reserves but v1 does not implement
// (PUT .../policy, POST .../archive). It still authenticates, so the route is
// honestly "recognized but not implemented" rather than a 404.
func (s *Server) handleReserved(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authn(w, r); !ok {
		return
	}
	writeError(w, http.StatusNotImplemented, protocol.CodeNotImplemented, "reserved in v1")
}

// authorizeServer authorizes a server-scoped capability against the reserved
// server resource (RFC-0019 §7.2). With no enforcer configured, authorization is
// disabled and everything is allowed (matching the data plane).
func (s *Server) authorizeServer(w http.ResponseWriter, id identity.Identity, cap authz.Capability) bool {
	return s.authorize(w, id, cap, authz.ServerResource)
}

// authorizeAny allows the request if the identity holds ANY of caps on repo
// (used by GET repo: admin OR read). On denial it writes 403.
func (s *Server) authorizeAny(w http.ResponseWriter, id identity.Identity, repo string, caps ...authz.Capability) bool {
	if s.authz == nil {
		return true
	}
	for _, c := range caps {
		if s.authz.Authorize(id.ID, c, repo) == nil {
			return true
		}
	}
	writeError(w, http.StatusForbidden, protocol.CodeUnauthorized,
		"subject "+id.ID+" lacks access to repository "+repo)
	return false
}

// writeManagerError maps a repomanager error to a control-plane status code
// (RFC-0019 §8.2).
func (s *Server) writeManagerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repomanager.ErrExists):
		writeError(w, http.StatusConflict, protocol.CodeRepoExists, err.Error())
	case errors.Is(err, repomanager.ErrNotFound):
		writeError(w, http.StatusNotFound, protocol.CodeUnknownRepo, err.Error())
	case errors.Is(err, repomanager.ErrInvalidName):
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, err.Error())
	}
}

// descriptorOf renders a manager Metadata as the wire descriptor (RFC-0019
// §6.1). The server owns this mapping so repomanager need not import protocol.
func descriptorOf(m *repomanager.Metadata) protocol.RepositoryDescriptor {
	return protocol.RepositoryDescriptor{
		ID:          m.ID,
		Name:        m.Name,
		Owner:       m.Owner,
		Visibility:  string(m.Visibility),
		State:       string(m.State),
		Description: m.Description,
		Archived:    m.State == repomanager.StateArchived,
		CreatedAt:   m.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   m.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
