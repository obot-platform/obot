// Links to a vMCP profile's agent setup, shared by the vMCP designer (which opens the profile) and the
// agent page (which links each readiness reason to the profile that fixes it).
//
//   /vmcps/{vmcpID}?view=profiles&profileID={profileID}#{anchor}
//
// profileID is the profile's server-assigned ID, not its name: the name is what the designer keeps in its
// own `profile` parameter, but the readiness API names profiles by ID. The hash, when present, is the ID of
// the field or button in the agent setup that fixes the problem.

export const PROFILE_ID_PARAM = 'profileID';
export const AGENT_SETUP_ANCHOR = 'agent-setup';

export interface ProfileSetupTarget {
	componentID: string;
	// A configuration key. Without it, the target is the component's OAuth grant.
	field?: string;
}

// The DOM ID of the input for a configuration key.
export function configFieldAnchor(componentID: string, key: string): string {
	return `agent-config-${componentID}-${key}`;
}

// The DOM ID of the Authorize button for a component's OAuth grant.
export function oauthAnchor(componentID: string): string {
	return `agent-oauth-${componentID}`;
}

export function profileSetupAnchor(target?: ProfileSetupTarget): string {
	if (!target) return AGENT_SETUP_ANCHOR;
	return target.field
		? configFieldAnchor(target.componentID, target.field)
		: oauthAnchor(target.componentID);
}

export function vmcpProfileHref(
	vmcpID: string,
	profileID: string,
	target?: ProfileSetupTarget
): `/${string}` {
	const query = new URLSearchParams({ view: 'profiles', [PROFILE_ID_PARAM]: profileID });
	return `/vmcps/${encodeURIComponent(vmcpID)}?${query}#${encodeURIComponent(profileSetupAnchor(target))}`;
}
