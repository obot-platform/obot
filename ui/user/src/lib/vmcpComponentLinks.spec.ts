import {
	vmcpComponentHref,
	vmcpComponentTarget,
	withoutVMcpComponentTarget
} from './vmcpComponentLinks';
import { describe, expect, it } from 'vitest';

describe('vmcpComponentHref', () => {
	it('opens the vMCP editor', () => {
		expect(vmcpComponentHref('vmcp1a')).toBe('/vmcps/vmcp1a');
	});

	it("opens a component's settings, or the OAuth or configuration in them", () => {
		expect(vmcpComponentHref('vmcp1a', { componentID: 'gh' })).toBe('/vmcps/vmcp1a?component=gh');
		expect(vmcpComponentHref('vmcp1a', { componentID: 'gh', settings: 'oauth' })).toBe(
			'/vmcps/vmcp1a?component=gh&settings=oauth'
		);
		expect(vmcpComponentHref('vmcp1a', { componentID: 'gh', settings: 'configuration' })).toBe(
			'/vmcps/vmcp1a?component=gh&settings=configuration'
		);
	});

	it('encodes IDs', () => {
		expect(vmcpComponentHref('a/b', { componentID: 'c&d', settings: 'oauth' })).toBe(
			'/vmcps/a%2Fb?component=c%26d&settings=oauth'
		);
	});
});

describe('vmcpComponentTarget', () => {
	it('reads back what a link names', () => {
		const url = new URL(
			vmcpComponentHref('vmcp1a', { componentID: 'c&d', settings: 'oauth' }),
			'http://localhost'
		);
		expect(vmcpComponentTarget(url)).toEqual({ componentID: 'c&d', settings: 'oauth' });
	});

	it('names no target without a component, and no settings it does not know', () => {
		expect(vmcpComponentTarget(new URL('http://localhost/vmcps/a?settings=oauth'))).toBeUndefined();
		expect(
			vmcpComponentTarget(new URL('http://localhost/vmcps/a?component=gh&settings=x'))
		).toEqual({ componentID: 'gh' });
	});

	it('drops the target and keeps everything else', () => {
		const url = new URL('http://localhost/vmcps/a?view=graph&component=gh&settings=oauth');
		expect(withoutVMcpComponentTarget(url).href).toBe('http://localhost/vmcps/a?view=graph');
		expect(url.searchParams.get('component')).toBe('gh');
	});
});
