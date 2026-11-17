import { configFieldAnchor, oauthAnchor, vmcpProfileHref } from './vmcpProfileLinks';
import { describe, expect, it } from 'vitest';

describe('vmcpProfileHref', () => {
	it('opens the profile by ID on the profiles view, at its agent setup', () => {
		expect(vmcpProfileHref('vmcp1a', 'vp1b')).toBe(
			'/vmcps/vmcp1a?view=profiles&profileID=vp1b#agent-setup'
		);
	});

	it('points at the field or the OAuth button that fixes a reason', () => {
		expect(vmcpProfileHref('vmcp1a', 'vp1b', { componentID: 'gh', field: 'API_KEY' })).toBe(
			`/vmcps/vmcp1a?view=profiles&profileID=vp1b#${encodeURIComponent(configFieldAnchor('gh', 'API_KEY'))}`
		);
		expect(vmcpProfileHref('vmcp1a', 'vp1b', { componentID: 'gh' })).toBe(
			`/vmcps/vmcp1a?view=profiles&profileID=vp1b#${encodeURIComponent(oauthAnchor('gh'))}`
		);
	});
});
