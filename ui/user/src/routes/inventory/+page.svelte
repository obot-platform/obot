<script lang="ts">
	import TabLayout from '$lib/components/TabLayout.svelte';
	import Devices from '$lib/components/admin/devices/Devices.svelte';
	import { profile } from '$lib/stores';
	import Configuration from './Configuration.svelte';
	import DeviceClients from './DeviceClients.svelte';
	import DeviceMcpServers from './DeviceMcpServers.svelte';
	import DeviceSkills from './DeviceSkills.svelte';
	import OverviewView from './OverviewView.svelte';
	import { untrack } from 'svelte';

	let { data } = $props();

	const defaultView = untrack(() =>
		profile.current.hasAdminAccess?.()
			? data.configuration
				? 'overview'
				: 'configuration'
			: 'devices'
	);

	let views = $derived([
		...(profile.current.hasAdminAccess?.()
			? [
					{ label: 'Overview', value: 'overview', content: overview },
					{ label: 'Configuration', value: 'configuration', content: configuration }
				]
			: []),
		{ label: 'Devices', value: 'devices', content: devices },
		...(profile.current.hasAdminAccess?.()
			? [
					{ label: 'Device Clients', value: 'device-clients', content: deviceClients },
					{ label: 'Device MCP Servers', value: 'device-mcp-servers', content: deviceMcpServers },
					{ label: 'Device Skills', value: 'device-skills', content: deviceSkills }
				]
			: [])
	]);
</script>

<svelte:head>
	<title>Obot | Inventory</title>
</svelte:head>

<TabLayout title="Inventory" {defaultView} classes={{ childrenContainer: 'max-w-none' }} {views} />

{#snippet overview()}
	<OverviewView stats={data.stats} range={data.range} />
{/snippet}

{#snippet configuration()}
	<Configuration
		configuration={data.configuration}
		enrollmentKeys={data.enrollmentKeys}
		assetSource={data.assetSource}
		assets={data.assets}
		assetLoadError={data.assetLoadError}
	/>
{/snippet}

{#snippet devices()}
	<Devices />
{/snippet}

{#snippet deviceClients()}
	<DeviceClients />
{/snippet}

{#snippet deviceMcpServers()}
	<DeviceMcpServers />
{/snippet}

{#snippet deviceSkills()}
	<DeviceSkills />
{/snippet}
