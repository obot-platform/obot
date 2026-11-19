import { componentAnchor, oauthAnchor, vmcpAgentSetupHref } from './vmcpAgentSetupLinks';
import { describe, expect, it } from 'vitest';

describe('vmcpAgentSetupHref', () => {
	it('opens the vMCP on its agent setup, not a profile', () => {
		expect(vmcpAgentSetupHref('vmcp1a')).toBe('/vmcps/vmcp1a?view=agent-setup#agent-setup');
	});

	it('points at the component or the OAuth button a reason is about', () => {
		expect(vmcpAgentSetupHref('vmcp1a', { componentID: 'gh', configuration: true })).toBe(
			`/vmcps/vmcp1a?view=agent-setup#${encodeURIComponent(componentAnchor('gh'))}`
		);
		expect(vmcpAgentSetupHref('vmcp1a', { componentID: 'gh' })).toBe(
			`/vmcps/vmcp1a?view=agent-setup#${encodeURIComponent(oauthAnchor('gh'))}`
		);
	});
});
