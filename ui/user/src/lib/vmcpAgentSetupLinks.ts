// Links to a vMCP's agent setup, shared by the vMCP designer (which shows it) and the agent page (which
// links each readiness reason to the place on the vMCP that fixes it).
//
//   /vmcps/{vmcpID}?view=agent-setup#{anchor}
//
// Agent setup belongs to the vMCP, never to a profile: agents get only values fixed on the vMCP or its
// catalog entry, and one OAuth grant per component serves every agent of the vMCP. The hash, when present,
// is the ID of the part of the agent setup that a problem is about: a component's OAuth button, or the
// component itself when it needs configuration that agents can't get.

export const AGENT_SETUP_VIEW = 'agent-setup';
export const AGENT_SETUP_ANCHOR = 'agent-setup';

export interface AgentSetupTarget {
	componentID: string;
	// The component needs configuration. Without it, the target is the component's OAuth grant.
	configuration?: boolean;
}

// The DOM ID of a component's section in the agent setup.
export function componentAnchor(componentID: string): string {
	return `agent-component-${componentID}`;
}

// The DOM ID of the Authorize button for a component's OAuth grant.
export function oauthAnchor(componentID: string): string {
	return `agent-oauth-${componentID}`;
}

export function agentSetupAnchor(target?: AgentSetupTarget): string {
	if (!target) return AGENT_SETUP_ANCHOR;
	return target.configuration
		? componentAnchor(target.componentID)
		: oauthAnchor(target.componentID);
}

export function vmcpAgentSetupHref(vmcpID: string, target?: AgentSetupTarget): `/${string}` {
	const query = new URLSearchParams({ view: AGENT_SETUP_VIEW });
	return `/vmcps/${encodeURIComponent(vmcpID)}?${query}#${encodeURIComponent(agentSetupAnchor(target))}`;
}
