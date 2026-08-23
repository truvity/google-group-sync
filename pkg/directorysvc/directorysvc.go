// Package directorysvc serves the ConnectRPC DirectoryService over the
// existing resolver: liveness and flat group membership for one corporate
// directory. Consumers (github-roster, zitadel-rbac-mapper) hold no
// directory credential — only this endpoint.
package directorysvc

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"connectrpc.com/connect"

	directoryv1 "github.com/truvity/google-group-sync/gen/directory/v1"
	"github.com/truvity/google-group-sync/gen/directory/v1/directoryv1connect"
	"github.com/truvity/google-group-sync/pkg/resolver"
)

// Server implements directoryv1connect.DirectoryServiceHandler by
// delegating to a resolver.GroupLister and applying the served-domain
// policy for account standing.
type Server struct {
	logger     *slog.Logger
	res        resolver.GroupLister
	domains    []string // served domains, lower-cased
	backend    string
	adminEmail string
	probeGroup string
}

var _ directoryv1connect.DirectoryServiceHandler = (*Server)(nil)

// New builds a DirectoryService handler. domains are the addresses this
// directory vouches for; adminEmail/probeGroup drive Probe.
func New(logger *slog.Logger, res resolver.GroupLister, domains []string, adminEmail, probeGroup string) *Server {
	lowered := make([]string, 0, len(domains))
	for _, d := range domains {
		lowered = append(lowered, strings.ToLower(strings.TrimSpace(d)))
	}

	return &Server{
		logger:     logger,
		res:        res,
		domains:    lowered,
		backend:    "google-workspace",
		adminEmail: adminEmail,
		probeGroup: probeGroup,
	}
}

// inDomain reports whether email's domain is one this directory serves.
func (s *Server) inDomain(email string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}

	dom := strings.ToLower(email[at+1:])
	for _, d := range s.domains {
		if d == dom {
			return true
		}
	}

	return false
}

func (s *Server) Describe(_ context.Context, _ *connect.Request[directoryv1.DescribeRequest]) (*connect.Response[directoryv1.DescribeResponse], error) {
	return connect.NewResponse(&directoryv1.DescribeResponse{
		Domains: s.domains,
		Backend: s.backend,
	}), nil
}

func (s *Server) Probe(ctx context.Context, _ *connect.Request[directoryv1.ProbeRequest]) (*connect.Response[directoryv1.ProbeResponse], error) {
	var err error
	if s.probeGroup != "" {
		_, err = s.res.GetGroup(ctx, s.probeGroup)
		// An absent probe group is a misconfiguration, but the credential
		// still worked — treat not-found as healthy-enough for the canary.
		if errors.Is(err, resolver.ErrGroupNotFound) {
			err = nil
		}
	} else if s.adminEmail != "" {
		_, err = s.res.GetAccount(ctx, s.adminEmail)
	}

	if err != nil {
		return connect.NewResponse(&directoryv1.ProbeResponse{Healthy: false, Detail: err.Error()}), nil
	}

	return connect.NewResponse(&directoryv1.ProbeResponse{Healthy: true}), nil
}

func (s *Server) GetGroup(ctx context.Context, req *connect.Request[directoryv1.GetGroupRequest]) (*connect.Response[directoryv1.GetGroupResponse], error) {
	g, err := s.res.GetGroup(ctx, req.Msg.GetEmail())
	if errors.Is(err, resolver.ErrGroupNotFound) {
		return connect.NewResponse(&directoryv1.GetGroupResponse{Found: false}), nil
	}

	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}

	return connect.NewResponse(&directoryv1.GetGroupResponse{
		Found: true,
		Group: &directoryv1.Group{Email: g.Email, Members: g.Members},
	}), nil
}

func (s *Server) ListGroups(ctx context.Context, _ *connect.Request[directoryv1.ListGroupsRequest]) (*connect.Response[directoryv1.ListGroupsResponse], error) {
	groups, err := s.res.ListGroups(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}

	out := make([]*directoryv1.Group, 0, len(groups))
	for i := range groups {
		out = append(out, &directoryv1.Group{Email: groups[i].Email, Members: groups[i].Members})
	}

	return connect.NewResponse(&directoryv1.ListGroupsResponse{Groups: out}), nil
}

// account resolves one address to its standing, applying the served-domain
// policy: an out-of-domain address is NO OPINION (in_domain=false), never a
// Google read.
func (s *Server) account(ctx context.Context, email string) (*directoryv1.Account, error) {
	if !s.inDomain(email) {
		return &directoryv1.Account{Email: email, InDomain: false}, nil
	}

	a, err := s.res.GetAccount(ctx, email)
	if err != nil {
		return nil, err
	}

	return &directoryv1.Account{
		Email:      email,
		InDomain:   true,
		Found:      a.Found,
		Live:       a.Live,
		GivenName:  a.GivenName,
		FamilyName: a.FamilyName,
	}, nil
}

func (s *Server) GetAccount(ctx context.Context, req *connect.Request[directoryv1.GetAccountRequest]) (*connect.Response[directoryv1.GetAccountResponse], error) {
	a, err := s.account(ctx, req.Msg.GetEmail())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}

	return connect.NewResponse(&directoryv1.GetAccountResponse{Account: a}), nil
}

func (s *Server) ResolveAccounts(ctx context.Context, req *connect.Request[directoryv1.ResolveAccountsRequest]) (*connect.Response[directoryv1.ResolveAccountsResponse], error) {
	emails := req.Msg.GetEmails()
	out := make([]*directoryv1.Account, 0, len(emails))

	for _, e := range emails {
		a, err := s.account(ctx, e)
		if err != nil {
			return nil, connect.NewError(connect.CodeUnavailable, err)
		}

		out = append(out, a)
	}

	return connect.NewResponse(&directoryv1.ResolveAccountsResponse{Accounts: out}), nil
}

func (s *Server) ResolveUser(ctx context.Context, req *connect.Request[directoryv1.ResolveUserRequest]) (*connect.Response[directoryv1.ResolveUserResponse], error) {
	ug, err := s.res.ResolveUser(ctx, req.Msg.GetEmail())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}

	return connect.NewResponse(&directoryv1.ResolveUserResponse{
		Groups:    ug.Groups,
		Suspended: ug.Suspended,
	}), nil
}
