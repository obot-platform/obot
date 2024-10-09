package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/otto8-ai/otto8/pkg/gateway/client"
	"github.com/otto8-ai/otto8/pkg/gateway/db"
	"github.com/otto8-ai/otto8/pkg/gateway/types"
	"gorm.io/gorm"
)

type Options struct {
	Hostname, UIHostname string
	GatewayDebug         bool
}

type Server struct {
	adminEmails    map[string]struct{}
	db             *db.DB
	baseURL, uiURL string
	httpClient     *http.Client
	client         *client.Client
}

func New(ctx context.Context, db *db.DB, adminEmails []string, opts Options) (*Server, error) {
	if err := db.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("auto migrate failed: %w", err)
	}

	if opts.GatewayDebug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	if opts.Hostname == "" {
		opts.Hostname = "http://localhost:8080"
	}
	if opts.UIHostname == "" {
		opts.UIHostname = opts.Hostname
	}

	if strings.HasPrefix(opts.Hostname, "localhost") || strings.HasPrefix(opts.Hostname, "127.0.0.1") {
		opts.Hostname = "http://" + opts.Hostname
	} else if !strings.HasPrefix(opts.Hostname, "http") {
		opts.Hostname = "https://" + opts.Hostname
	}
	if !strings.HasPrefix(opts.UIHostname, "http") {
		opts.UIHostname = "https://" + opts.UIHostname
	}

	adminEmailsSet := make(map[string]struct{}, len(adminEmails))
	for _, email := range adminEmails {
		adminEmailsSet[email] = struct{}{}
	}

	s := &Server{
		adminEmails: adminEmailsSet,
		db:          db,
		baseURL:     opts.Hostname,
		uiURL:       opts.UIHostname,
		httpClient:  &http.Client{},
		client:      client.New(db, adminEmails),
	}

	go s.autoCleanupTokens(ctx)
	go s.oAuthCleanup(ctx)

	return s, nil
}

func (s *Server) UpsertAuthProvider(ctx context.Context, clientID, clientSecret string) (uint, error) {
	if clientID == "" || clientSecret == "" {
		return 0, nil
	}

	authProvider := &types.AuthProvider{
		Type:          types.AuthTypeGoogle,
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		OAuthURL:      types.GoogleOAuthURL,
		JWKSURL:       types.GoogleJWKSURL,
		ServiceName:   "Google",
		Scopes:        "openid profile email",
		UsernameClaim: "username",
		EmailClaim:    "email",
		Slug:          "google",
		Expiration:    "7d",
		ExpirationDur: 7 * 24 * time.Hour,
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing := new(types.AuthProvider)
		if err := tx.WithContext(ctx).Where("slug = ?", authProvider.Slug).First(existing).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if existing.ID == 0 {
			return tx.WithContext(ctx).Create(authProvider).Error
		}

		authProvider.Model = existing.Model
		return tx.WithContext(ctx).Model(authProvider).Updates(authProvider).Error
	}); err != nil {
		return 0, err
	}

	return authProvider.ID, nil
}
