// Package localconfig defines the setup-managed local CLI config
// boundary.
//
// The first behavior-changing phase will add URL normalization and XDG
// file persistence here. Phase 0 only establishes the shared config
// shape so command code has a single boundary to depend on later.
package localconfig
