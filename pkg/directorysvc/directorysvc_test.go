package directorysvc

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"connectrpc.com/connect"

	directoryv1 "github.com/truvity/google-group-sync/gen/directory/v1"
	"github.com/truvity/google-group-sync/pkg/resolver"
)

// fakeResolver is a resolver.GroupLister for tests.
type fakeResolver struct {
	accounts map[string]resolver.Account
	groups   map[string]*resolver.Group
}

func (f *fakeResolver) ResolveGroups(context.Context, string) ([]string, error) { return nil, nil }
func (f *fakeResolver) ResolveUser(context.Context, string) (resolver.UserGroups, error) {
	return resolver.UserGroups{}, nil
}
func (f *fakeResolver) ListGroups(context.Context) ([]resolver.Group, error) { return nil, nil }
func (f *fakeResolver) GetGroup(_ context.Context, email string) (*resolver.Group, error) {
	g, ok := f.groups[email]
	if !ok {
		return nil, resolver.ErrGroupNotFound
	}
	return g, nil
}
func (f *fakeResolver) GetAccount(_ context.Context, email string) (resolver.Account, error) {
	a, ok := f.accounts[email]
	if !ok {
		return resolver.Account{Email: email, Found: false}, nil
	}
	return a, nil
}

func newServer(f *fakeResolver) *Server {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), f, []string{"acme.example"}, "admin@acme.example", "")
}

// An in-domain live account resolves with its standing and name.
func TestGetAccount_InDomainLive(t *testing.T) {
	f := &fakeResolver{accounts: map[string]resolver.Account{
		"dana@acme.example": {Email: "dana@acme.example", Found: true, Live: true, GivenName: "Dana", FamilyName: "Okafor"},
	}}
	resp, err := newServer(f).GetAccount(context.Background(), connect.NewRequest(&directoryv1.GetAccountRequest{Email: "dana@acme.example"}))
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	a := resp.Msg.GetAccount()
	if !a.GetInDomain() || !a.GetFound() || !a.GetLive() || a.GetGivenName() != "Dana" {
		t.Fatalf("unexpected account: %+v", a)
	}
}

// An out-of-domain address is NO OPINION — in_domain=false and the resolver
// is never consulted (fail-safe: never read as "gone").
func TestGetAccount_OutOfDomainIsNoOpinion(t *testing.T) {
	f := &fakeResolver{accounts: map[string]resolver.Account{}}
	resp, err := newServer(f).GetAccount(context.Background(), connect.NewRequest(&directoryv1.GetAccountRequest{Email: "someone@other.example"}))
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	a := resp.Msg.GetAccount()
	if a.GetInDomain() {
		t.Fatalf("out-of-domain must be in_domain=false, got %+v", a)
	}
}

// An in-domain address the directory does not know is found=false (gone),
// distinct from out-of-domain no-opinion.
func TestGetAccount_InDomainNotFound(t *testing.T) {
	f := &fakeResolver{accounts: map[string]resolver.Account{}}
	resp, _ := newServer(f).GetAccount(context.Background(), connect.NewRequest(&directoryv1.GetAccountRequest{Email: "ghost@acme.example"}))
	a := resp.Msg.GetAccount()
	if !a.GetInDomain() || a.GetFound() {
		t.Fatalf("in-domain unknown must be in_domain=true, found=false, got %+v", a)
	}
}

// A missing group is found=false, not an error (absent group → fail-safe).
func TestGetGroup_NotFound(t *testing.T) {
	f := &fakeResolver{groups: map[string]*resolver.Group{}}
	resp, err := newServer(f).GetGroup(context.Background(), connect.NewRequest(&directoryv1.GetGroupRequest{Email: "nope@acme.example"}))
	if err != nil {
		t.Fatalf("GetGroup: %v", err)
	}
	if resp.Msg.GetFound() {
		t.Fatalf("missing group must be found=false")
	}
}
