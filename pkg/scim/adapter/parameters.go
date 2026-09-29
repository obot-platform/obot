package adapter

import (
	"slices"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

// Parameters are an auth provider's effective configuration parameters, which depend on its SCIM setup. Every check
// of whether the provider is configured uses Required, and the configuration form shows Required and Optional.
type Parameters struct {
	Required []types2.ProviderConfigurationParameter
	Optional []types2.ProviderConfigurationParameter
	// Dropped names the parameters that a submitted configuration must not keep.
	Dropped []string
	// Together names optional parameters that a configuration must hold all of or none of, because the auth
	// provider refuses to start with only some of them.
	Together []string
}

// EffectiveParameters returns an auth provider's effective configuration parameters, from its manifest, its SCIM
// connection, which is nil when it has none, the connection's adapter, and its stored credential, which is nil when it
// has none.
//
// Only the adapter's directory parameters differ from the manifest:
//   - Without a connection, the provider synchronizes its directory at sign-in, so they are required as the manifest
//     lists them.
//   - With a connection, they are never required. If the stored credential still holds them, they are optional and
//     described as unused, so they can be removed. Otherwise they are absent, and dropped if submitted.
func EffectiveParameters(manifest types2.AuthProviderManifest, conn *types.SCIMConnection, stored map[string]string) Parameters {
	params := Parameters{
		Required: slices.Clone(manifest.RequiredConfigurationParameters),
		Optional: slices.Clone(manifest.OptionalConfigurationParameters),
	}
	if conn == nil {
		return params
	}

	a, ok := Lookup(conn.AdapterType)
	if !ok {
		// A connection whose rules are unknown cannot relax anything, so the provider needs everything its manifest
		// requires.
		return params
	}

	directory := a.DirectoryParameters()
	isDirectory := func(p types2.ProviderConfigurationParameter) bool {
		return slices.ContainsFunc(directory, func(d DirectoryParameter) bool { return d.Name == p.Name })
	}
	stillStored := slices.ContainsFunc(directory, func(d DirectoryParameter) bool {
		return stored[d.Name] != ""
	})

	params.Required = slices.DeleteFunc(params.Required, isDirectory)
	params.Optional = slices.DeleteFunc(params.Optional, isDirectory)
	for _, d := range directory {
		if !stillStored {
			params.Dropped = append(params.Dropped, d.Name)
			continue
		}

		params.Together = append(params.Together, d.Name)

		param := types2.ProviderConfigurationParameter{
			Name:        d.Name,
			Description: d.UnusedDescription,
		}
		if i := slices.IndexFunc(manifest.RequiredConfigurationParameters, func(p types2.ProviderConfigurationParameter) bool { return p.Name == d.Name }); i >= 0 {
			param = manifest.RequiredConfigurationParameters[i]
			param.Description = d.UnusedDescription
		}
		params.Optional = append(params.Optional, param)
	}

	return params
}

// IncompleteGroup returns the parameters of Together that config lacks when it holds some but not all of them, and
// nothing otherwise. Empty values count as absent.
func (p Parameters) IncompleteGroup(config map[string]string) []string {
	var missing []string
	for _, name := range p.Together {
		if config[name] == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) == len(p.Together) {
		return nil
	}
	return missing
}

// SupportsSCIM reports whether the named auth provider can have a SCIM connection: the registry has an adapter for
// it, and its manifest declares a group ID prefix, which groups created through SCIM keep.
func SupportsSCIM(authProviderName string, manifest types2.AuthProviderManifest) bool {
	_, ok := ForAuthProvider(authProviderName)
	return ok && manifest.GroupIDPrefix != ""
}
