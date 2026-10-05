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

// ProviderState is what an auth provider's effective configuration parameters depend on, besides its manifest.
type ProviderState struct {
	// AuthProviderName names the auth provider. Without a SCIM connection, it selects the provider's adapter.
	AuthProviderName string
	// Configured is set when the configuration being described is the provider's active one: for the configured
	// auth provider, and for every check of whether a stored configuration is complete. Without a SCIM connection,
	// such a provider synchronizes its directory at sign-in.
	Configured bool
	// ConnectionAdapterType is the adapter type of the provider's SCIM connection, or empty when it has none.
	ConnectionAdapterType string
	// Stored is the provider's stored configuration, or nil when it has none.
	Stored map[string]string
}

// ConnectionAdapterType returns the adapter type of conn, an auth provider's SCIM connection, or an empty string when
// conn is nil, as ProviderState.ConnectionAdapterType takes it.
func ConnectionAdapterType(conn *types.SCIMConnection) string {
	if conn == nil {
		return ""
	}
	return conn.AdapterType
}

// EffectiveParameters returns an auth provider's effective configuration parameters, from its manifest, its adapter,
// its SCIM connection, and its stored configuration.
//
// Only the adapter's directory parameters differ from the manifest:
//   - A provider that is being configured or staged, and has no connection, may omit them: providing them sets up
//     directory synchronization, and omitting them sets up SCIM. They are optional, described as that choice, and
//     must be provided together.
//   - A configured provider without a connection synchronizes its directory at sign-in, so they are required as the
//     manifest lists them.
//   - With a connection, they are never required. If the stored configuration still holds them, they are optional
//     and described as unused, so they can be removed. Otherwise they are absent, and dropped if submitted.
func EffectiveParameters(manifest types2.AuthProviderManifest, state ProviderState) Parameters {
	params := Parameters{
		Required: slices.Clone(manifest.RequiredConfigurationParameters),
		Optional: slices.Clone(manifest.OptionalConfigurationParameters),
	}

	var (
		a  Adapter
		ok bool
	)
	switch {
	case state.ConnectionAdapterType != "":
		a, ok = Lookup(state.ConnectionAdapterType)
	case !state.Configured && SupportsSCIM(state.AuthProviderName, manifest):
		a, ok = ForAuthProvider(state.AuthProviderName)
	}
	if !ok {
		// Without a connection, a configured provider synchronizes its directory, and a provider without an adapter
		// has nothing to relax. A connection whose rules are unknown cannot relax anything either, so the provider
		// needs everything its manifest requires.
		return params
	}

	directory := a.DirectoryParameters()
	isDirectory := func(p types2.ProviderConfigurationParameter) bool {
		return slices.ContainsFunc(directory, func(d DirectoryParameter) bool { return d.Name == p.Name })
	}
	params.Required = slices.DeleteFunc(params.Required, isDirectory)
	params.Optional = slices.DeleteFunc(params.Optional, isDirectory)

	if state.ConnectionAdapterType == "" {
		params.Together = make([]string, 0, len(directory))
		params.Optional = slices.Grow(params.Optional, len(directory))
		for _, d := range directory {
			params.Together = append(params.Together, d.Name)
			params.Optional = append(params.Optional, directoryParameter(manifest, d, d.SetupDescription))
		}
		return params
	}

	stillStored := slices.ContainsFunc(directory, func(d DirectoryParameter) bool {
		return state.Stored[d.Name] != ""
	})
	if !stillStored {
		params.Dropped = make([]string, 0, len(directory))
		for _, d := range directory {
			params.Dropped = append(params.Dropped, d.Name)
		}
		return params
	}

	params.Together = make([]string, 0, len(directory))
	params.Optional = slices.Grow(params.Optional, len(directory))
	for _, d := range directory {
		params.Together = append(params.Together, d.Name)
		params.Optional = append(params.Optional, directoryParameter(manifest, d, d.UnusedDescription))
	}

	return params
}

// directoryParameter returns the manifest's definition of a directory parameter with the given description.
func directoryParameter(manifest types2.AuthProviderManifest, d DirectoryParameter, description string) types2.ProviderConfigurationParameter {
	param := types2.ProviderConfigurationParameter{
		Name: d.Name,
	}
	for _, list := range [][]types2.ProviderConfigurationParameter{manifest.RequiredConfigurationParameters, manifest.OptionalConfigurationParameters} {
		if i := slices.IndexFunc(list, func(p types2.ProviderConfigurationParameter) bool { return p.Name == d.Name }); i >= 0 {
			param = list[i]
			break
		}
	}
	param.Description = description
	return param
}

// DirectoryParametersProvided reports whether config holds any of the adapter's directory parameters. Empty values
// count as absent. Providing them chooses directory synchronization, and omitting them all chooses SCIM.
func DirectoryParametersProvided(a Adapter, config map[string]string) bool {
	return slices.ContainsFunc(a.DirectoryParameters(), func(d DirectoryParameter) bool {
		return config[d.Name] != ""
	})
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
