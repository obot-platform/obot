import { getMcpTunnelConnectionsKey, isMcpTunnelDisconnected } from './mcpTunnel.ts';
import assert from 'node:assert/strict';
import test from 'node:test';

const remoteServer = {
	manifest: {
		runtime: 'remote',
		remoteConfig: {
			tunnelName: 'mt1office'
		}
	}
};

test('reports a configured remote server when its tunnel disconnects', () => {
	assert.equal(isMcpTunnelDisconnected(remoteServer, []), true);
	assert.equal(isMcpTunnelDisconnected(remoteServer, [{ name: 'mt1office' }]), false);
});

test('does not report a disconnected tunnel when status is unavailable or unused', () => {
	assert.equal(isMcpTunnelDisconnected(remoteServer, undefined), false);
	assert.equal(
		isMcpTunnelDisconnected(
			{
				manifest: {
					runtime: 'remote',
					remoteConfig: {}
				}
			},
			[]
		),
		false
	);
	assert.equal(
		isMcpTunnelDisconnected(
			{
				manifest: {
					runtime: 'containerized'
				}
			},
			[]
		),
		false
	);
});

test('changes the table remeasurement key only when tunnel connectivity changes', () => {
	assert.equal(getMcpTunnelConnectionsKey(undefined), 'unknown');
	assert.equal(
		getMcpTunnelConnectionsKey([{ name: 'mt1office' }, { name: 'mt1lab' }]),
		getMcpTunnelConnectionsKey([{ name: 'mt1lab' }, { name: 'mt1office' }])
	);
	assert.notEqual(
		getMcpTunnelConnectionsKey([{ name: 'mt1office' }]),
		getMcpTunnelConnectionsKey([])
	);
});
