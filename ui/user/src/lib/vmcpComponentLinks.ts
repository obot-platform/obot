// Links to a component's settings in the vMCP editor, shared by the designer (which opens them) and the
// agent page (which links each readiness reason to the place on the vMCP that fixes it).
//
//   /vmcps/{vmcpID}?component={componentID}&settings={oauth|configuration}
//
// A component's settings are what its card opens in the designer: the list of what can be done with
// the component, each in its own dialog. `settings` opens one of those dialogs directly: the
// component's preconfigured OAuth, or its configuration. Without it, the list opens.

const VMCP_COMPONENT_PARAM = 'component';
const VMCP_COMPONENT_SETTINGS_PARAM = 'settings';

export type VMcpComponentSettings = 'oauth' | 'configuration';

export interface VMcpComponentTarget {
	componentID: string;
	settings?: VMcpComponentSettings;
}

// The vMCP's editor, open on a component's settings when a target is given.
export function vmcpComponentHref(vmcpID: string, target?: VMcpComponentTarget): `/${string}` {
	const path = `/vmcps/${encodeURIComponent(vmcpID)}` as const;
	if (!target) return path;
	const query = new URLSearchParams({ [VMCP_COMPONENT_PARAM]: target.componentID });
	if (target.settings) query.set(VMCP_COMPONENT_SETTINGS_PARAM, target.settings);
	return `${path}?${query}`;
}

// The component whose settings a URL asks the designer to open, if any. An unknown `settings` opens
// the component's list of settings.
export function vmcpComponentTarget(url: {
	searchParams: Pick<URLSearchParams, 'get'>;
}): VMcpComponentTarget | undefined {
	const componentID = url.searchParams.get(VMCP_COMPONENT_PARAM);
	if (!componentID) return undefined;
	const settings = url.searchParams.get(VMCP_COMPONENT_SETTINGS_PARAM);
	return settings === 'oauth' || settings === 'configuration'
		? { componentID, settings }
		: { componentID };
}

// A copy of the URL without a component target, for when its settings close.
export function withoutVMcpComponentTarget(url: URL | { toString(): string }): URL {
	const next = new URL(url.toString());
	next.searchParams.delete(VMCP_COMPONENT_PARAM);
	next.searchParams.delete(VMCP_COMPONENT_SETTINGS_PARAM);
	return next;
}
