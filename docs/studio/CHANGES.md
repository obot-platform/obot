# OIDC JWT Authenticator - Upstream Touchpoints

This manifest tracks every file outside `pkg/oidcjwt/` that the OIDC JWT
authenticator integration touches. Run
`scripts/check-upstream-touchpoints.sh` after each rebase.

## Allowed touchpoints

| File | Why |
|---|---|
| `pkg/services/config.go` | Append `oidcjwt.NewAuthenticator(...)` to the authenticator union when enabled. |
| `chart/values.yaml` | Add new env-var keys under the existing `config:` map. |
| `go.mod`, `go.sum` | New dependency: `github.com/coreos/go-oidc/v3`. |
| `docs/design/oidc-jwt-authn/README.md` | Design document. |
| `docs/plans/2026-06-12-oidc-jwt-authn.md` | Implementation plan. |
| `docs/studio/CHANGES.md` | This manifest. |
| `scripts/check-upstream-touchpoints.sh` | CI check. |

All other code lives under `pkg/oidcjwt/` and is additive.
