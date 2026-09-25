package mcpconnect

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"golang.org/x/oauth2"
)

type storedToken struct {
	Config *oauth2.Config
	Token  *oauth2.Token
}

type tokenStore struct{ path string }

type sharedTokenSource struct {
	ctx   context.Context
	store *tokenStore
}

// Lock a separate, stable file: the credential file is atomically replaced.
// Never unlink the lock file, which could let processes lock different inodes.
func (s *tokenStore) lock(ctx context.Context) (func(), error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	lock := flock.New(s.path+".lock", flock.SetPermissions(0o600))
	locked, err := lock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil || !locked {
		_ = lock.Close()
		if err == nil {
			err = ctx.Err()
		}
		return nil, fmt.Errorf("lock Obot OAuth credentials: %w", err)
	}
	return func() { _ = lock.Close() }, nil
}

func newTokenStore(dir, connectURL string) *tokenStore {
	return &tokenStore{path: filepath.Join(dir, fmt.Sprintf("%x.json", sha256.Sum256([]byte(connectURL))))}
}

func (s *tokenStore) read() (*storedToken, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var stored storedToken
	if err := json.Unmarshal(b, &stored); err != nil {
		return nil, fmt.Errorf("read cached Obot OAuth credentials: %w", err)
	}
	if stored.Config == nil || stored.Token == nil {
		return nil, fmt.Errorf("incomplete cached Obot OAuth credentials")
	}
	return &stored, nil
}

func (s *tokenStore) save(conf *oauth2.Config, token *oauth2.Token) error {
	b, err := json.Marshal(storedToken{Config: conf, Token: token})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".oauth-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}

func (s *tokenStore) load(ctx context.Context, client *http.Client) (oauth2.TokenSource, error) {
	stored, err := s.read()
	if err != nil || stored == nil {
		return nil, err
	}
	return &sharedTokenSource{ctx: context.WithValue(ctx, oauth2.HTTPClient, client), store: s}, nil
}

func (s *sharedTokenSource) Token() (*oauth2.Token, error) {
	unlock, err := s.store.lock(s.ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	return s.store.tokenLocked(s.ctx)
}

// tokenLocked reloads before refreshing so another process's rotated refresh
// token or completed login is always used. The caller must hold the file lock.
func (s *tokenStore) tokenLocked(ctx context.Context) (*oauth2.Token, error) {
	stored, err := s.read()
	if err != nil {
		return nil, err
	}
	if stored == nil || stored.Token.AccessToken == "" {
		return &oauth2.Token{TokenType: "Bearer"}, nil
	}
	tok, err := stored.Config.TokenSource(ctx, stored.Token).Token()
	if err != nil {
		var retrieve *oauth2.RetrieveError
		if !errors.As(err, &retrieve) || (retrieve.ErrorCode != "invalid_grant" && retrieve.ErrorCode != "invalid_client") {
			return nil, err
		}
		// Invalidate for all processes, then let the next challenge start login.
		tok = &oauth2.Token{TokenType: "Bearer"}
	}
	if tok.AccessToken != stored.Token.AccessToken || tok.RefreshToken != stored.Token.RefreshToken || !tok.Expiry.Equal(stored.Token.Expiry) {
		if err := s.save(stored.Config, tok); err != nil {
			return nil, err
		}
	}
	return tok, nil
}
