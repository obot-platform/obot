import type { VMCP, VMCPInstance } from '$lib/services';
import { userAllowedConfigurationKeys, vmcpTesterServer } from './tester';
import { describe, expect, it } from 'vitest';

function vmcp(): VMCP {
	return {
		id: 'vmcp1-test',
		created: '2026-02-01T00:00:00Z',
		displayName: 'Virtual test server',
		description: 'Aggregates test tools',
		icon: 'https://example.com/vmcp.png',
		components: [
			{
				id: 'component-1',
				name: 'Component one',
				mcpCatalogID: 'default',
				mcpServerCatalogEntryID: 'entry-1',
				catalogEntry: { manifest: { name: 'Component one', runtime: 'npx' } }
			}
		],
		status: { ready: true }
	};
}

function vmcpInstance(): VMCPInstance {
	return {
		id: 'vmcpi1-test',
		created: '2026-02-02T00:00:00Z',
		userID: 'user-1',
		vmcpID: 'vmcp1-test',
		status: { configured: true }
	};
}

describe('vmcpTesterServer', () => {
	it('keeps the requested connect ID as the connection target', () => {
		const target = vmcp();

		expect(vmcpTesterServer(target, target.id)).toMatchObject({
			id: target.id,
			configured: true,
			deploymentStatus: 'Available',
			canConnect: true,
			manifest: {
				name: target.displayName,
				description: target.description,
				runtime: 'vmcp'
			}
		});
	});

	it('treats a vMCP without an instance as setup-required when components need user configuration', () => {
		const target = vmcp();
		target.components[0].configuration = [{ key: 'API_TOKEN', policy: 'userAllowed' }];

		expect(userAllowedConfigurationKeys(target)).toEqual(['component-1.API_TOKEN']);
		expect(vmcpTesterServer(target, target.id)).toMatchObject({
			id: target.id,
			configured: false,
			missingRequiredEnvVars: ['component-1.API_TOKEN']
		});
	});

	it('attaches the supplied instance configuration status without changing the connect ID', () => {
		const target = vmcp();
		const instance = {
			...vmcpInstance(),
			status: {
				configured: false,
				missingRequiredConfiguration: ['component-1.API_KEY']
			}
		};

		expect(vmcpTesterServer(target, target.id, instance)).toMatchObject({
			id: target.id,
			userID: instance.userID,
			configured: false,
			missingRequiredEnvVars: ['component-1.API_KEY'],
			manifest: { name: target.displayName, runtime: 'vmcp' }
		});
	});

	it('can connect through a vMCP instance ID when that is the requested target', () => {
		const target = vmcp();
		const instance = {
			...vmcpInstance(),
			status: {
				configured: false,
				missingRequiredConfiguration: ['component-1.API_KEY']
			}
		};

		expect(vmcpTesterServer(target, instance.id, instance)).toMatchObject({
			id: instance.id,
			userID: instance.userID,
			configured: false,
			missingRequiredEnvVars: ['component-1.API_KEY']
		});
	});
});
