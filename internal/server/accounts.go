package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/identity"
	"github.com/thulasiramk-2310/vara/internal/protocol"
)

// This file is the RFC-0020 account/session/token control plane. Like the
// RFC-0019 repository control plane it reuses the server's authentication
// preamble (authn) and authorization enforcer, and delegates all state to the
// identity layer's AccountManager. Secrets are returned exactly once, at
// creation, and never logged (§6).

// handleLogin: POST /_vara/sessions. The ONE unauthenticated control-plane route
// (it is the authentication). A failed login is an indistinguishable 401 (§12).
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	// Run the preamble WITHOUT requiring an identity — login establishes one.
	echoHeaders(w, r)
	var req protocol.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "decode login: "+err.Error())
		return
	}
	secret, expiresAt, err := s.accounts.Login(req.Username, req.Password)
	if err != nil {
		// Always the same 401 regardless of absent/disabled/wrong-password (§12).
		writeError(w, http.StatusUnauthorized, protocol.CodeUnauthenticated, "invalid credentials")
		return
	}
	writeJSON(w, http.StatusCreated, protocol.LoginResponse{
		Secret: secret, ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
	})
}

// handleLogout: DELETE /_vara/sessions/current. Revokes the session the caller
// presented (its bearer secret), immediately (§5.4).
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authn(w, r); !ok {
		return
	}
	// The caller must present the session as a bearer credential to revoke it.
	cred, err := identity.ParseHeader(r.Header.Get("Authorization"))
	if err != nil || cred == nil || cred.Scheme != identity.SchemeBearer {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "logout requires the session bearer token")
		return
	}
	if err := s.accounts.Logout(cred.Token); err != nil {
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCreateToken: POST /_vara/tokens. Mints a token for the CALLER's account.
func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authn(w, r)
	if !ok {
		return
	}
	if id.ID == identity.Anonymous.ID {
		writeError(w, http.StatusUnauthorized, protocol.CodeUnauthenticated, "authentication required to mint a token")
		return
	}
	var req protocol.CreateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "decode token request: "+err.Error())
		return
	}
	tokenID, secret, err := s.accounts.CreateToken(id.ID, req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, protocol.CreateTokenResponse{
		ID: tokenID, Name: req.Name, Secret: secret, CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// handleListTokens: GET /_vara/tokens. Lists the CALLER's tokens, metadata only.
func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authn(w, r)
	if !ok {
		return
	}
	if id.ID == identity.Anonymous.ID {
		writeError(w, http.StatusUnauthorized, protocol.CodeUnauthenticated, "authentication required")
		return
	}
	infos, err := s.accounts.ListTokens(id.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, err.Error())
		return
	}
	resp := protocol.ListTokensResponse{Tokens: make([]protocol.TokenInfo, 0, len(infos))}
	for _, t := range infos {
		ti := protocol.TokenInfo{ID: t.ID, Name: t.Name, CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339)}
		if !t.LastUsedAt.IsZero() {
			ti.LastUsedAt = t.LastUsedAt.UTC().Format(time.RFC3339)
		}
		resp.Tokens = append(resp.Tokens, ti)
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleRevokeToken: DELETE /_vara/tokens/{id}. Revokes one of the CALLER's own
// tokens (a caller manages only its own).
func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authn(w, r)
	if !ok {
		return
	}
	if id.ID == identity.Anonymous.ID {
		writeError(w, http.StatusUnauthorized, protocol.CodeUnauthenticated, "authentication required")
		return
	}
	if err := s.accounts.RevokeToken(id.ID, r.PathValue("id")); err != nil {
		s.writeAccountError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCreateAccount: POST /_vara/accounts. Requires manage-accounts (§8.3).
func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authn(w, r)
	if !ok {
		return
	}
	if !s.authorizeServer(w, id, authz.CapManageAccounts) {
		return
	}
	var req protocol.CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "decode account request: "+err.Error())
		return
	}
	if err := s.accounts.CreateAccount(req.Username, req.Password); err != nil {
		s.writeAccountError(w, err)
		return
	}
	a, err := s.accounts.Account(req.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, accountDescriptorOf(a))
}

// handleDeleteAccount: DELETE /_vara/accounts/{username}. Requires manage-accounts.
func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authn(w, r)
	if !ok {
		return
	}
	if !s.authorizeServer(w, id, authz.CapManageAccounts) {
		return
	}
	if err := s.accounts.DeleteAccount(r.PathValue("username")); err != nil {
		s.writeAccountError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDisableAccount: POST /_vara/accounts/{username}/disable. Requires
// manage-accounts. Revokes all the account's credentials (§5.4).
func (s *Server) handleDisableAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authn(w, r)
	if !ok {
		return
	}
	if !s.authorizeServer(w, id, authz.CapManageAccounts) {
		return
	}
	if err := s.accounts.DisableAccount(r.PathValue("username")); err != nil {
		s.writeAccountError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSetPassword: PUT /_vara/accounts/{username}/password. An account may
// change its OWN password when authenticated; otherwise manage-accounts is
// required (§8.3).
func (s *Server) handleSetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authn(w, r)
	if !ok {
		return
	}
	username := r.PathValue("username")
	if id.ID != username {
		// Not self → needs manage-accounts.
		if !s.authorizeServer(w, id, authz.CapManageAccounts) {
			return
		}
	} else if id.ID == identity.Anonymous.ID {
		writeError(w, http.StatusUnauthorized, protocol.CodeUnauthenticated, "authentication required")
		return
	}
	var req protocol.SetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "decode password request: "+err.Error())
		return
	}
	if err := s.accounts.ChangePassword(username, req.Password); err != nil {
		s.writeAccountError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// repoScopeCaps / serverScopeCaps are the capability sets whoami reports for a
// repository resource and the server resource, respectively.
var repoScopeCaps = []authz.Capability{
	authz.CapRead, authz.CapCreateRef, authz.CapPush, authz.CapForcePush,
	authz.CapDeleteRef, authz.CapDeleteRepo, authz.CapRenameRepo, authz.CapAdmin,
}

var serverScopeCaps = []authz.Capability{
	authz.CapCreateRepo, authz.CapListRepos, authz.CapManageAccounts,
}

// handleWhoami: GET /_vara/whoami[?repo=<name>]. Reports the resolved identity,
// and (with ?repo) which capabilities it holds there — a read-only view of the
// RFC-0018 decision, for debugging permissions. Always available.
func (s *Server) handleWhoami(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authn(w, r)
	if !ok {
		return
	}
	resp := protocol.WhoamiResponse{
		ID:        id.ID,
		Method:    id.Method.String(),
		Anonymous: id.ID == identity.Anonymous.ID,
	}
	if repo := r.URL.Query().Get("repo"); repo != "" {
		resp.Repository = repo
		caps := repoScopeCaps
		if repo == authz.ServerResource {
			caps = serverScopeCaps
		}
		resp.Capabilities = make(map[string]bool, len(caps))
		for _, c := range caps {
			resp.Capabilities[string(c)] = s.hasCapability(id, c, repo)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// hasCapability reports whether id holds cap on repo. With no enforcer configured,
// authorization is disabled and everything is granted (matching request handling).
func (s *Server) hasCapability(id identity.Identity, cap authz.Capability, repo string) bool {
	if s.authz == nil {
		return true
	}
	return s.authz.Authorize(id.ID, cap, repo) == nil
}

// writeAccountError maps an AccountManager error to a control-plane status code
// (RFC-0020 §8.4).
func (s *Server) writeAccountError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrAccountExists):
		writeError(w, http.StatusConflict, protocol.CodeAccountExists, err.Error())
	case errors.Is(err, identity.ErrNoAccount), errors.Is(err, identity.ErrNoCredential):
		writeError(w, http.StatusNotFound, protocol.CodeNotFound, err.Error())
	case errors.Is(err, identity.ErrWeakPassword):
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, err.Error())
	}
}

func accountDescriptorOf(a *identity.Account) protocol.AccountDescriptor {
	return protocol.AccountDescriptor{
		Username:  a.Username,
		State:     string(a.State),
		CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339),
	}
}
