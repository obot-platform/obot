// Package credentials defines the CLI credential storage boundary.
//
// Phase 0 intentionally does not migrate or read the existing XDG token
// file used by pkg/cli/internal/token.go. That legacy file remains the
// active runtime behavior until the keyring-backed authentication phase
// switches callers to Store.
package credentials
