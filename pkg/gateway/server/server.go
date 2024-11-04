package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/otto8-ai/otto8/pkg/gateway/client"
	"github.com/otto8-ai/otto8/pkg/gateway/db"
	"github.com/otto8-ai/otto8/pkg/gateway/server/dispatcher"
	"github.com/otto8-ai/otto8/pkg/gateway/types"
	"github.com/otto8-ai/otto8/pkg/invoke"
	"github.com/otto8-ai/otto8/pkg/jwt"
	"gorm.io/gorm"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Options struct {
	Hostname, UIHostname string
	GatewayDebug         bool
}

type Server struct {
	adminEmails     map[string]struct{}
	db              *db.DB
	baseURL, uiURL  string
	httpClient      *http.Client
	client          *client.Client
	tokenService    *jwt.TokenService
	modelDispatcher *dispatcher.Dispatcher
}

func New(ctx context.Context, db *db.DB, c kclient.Client, invoker *invoke.Invoker, tokenService *jwt.TokenService, adminEmails []string, opts Options) (*Server, error) {
	if err := db.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("auto migrate failed: %w", err)
	}

	adminEmailsSet := make(map[string]struct{}, len(adminEmails))
	for _, email := range adminEmails {
		adminEmailsSet[email] = struct{}{}
	}

	s := &Server{
		adminEmails:     adminEmailsSet,
		db:              db,
		baseURL:         opts.Hostname,
		uiURL:           opts.UIHostname,
		httpClient:      &http.Client{},
		client:          client.New(db, adminEmails),
		tokenService:    tokenService,
		modelDispatcher: dispatcher.New(invoker, c),
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
