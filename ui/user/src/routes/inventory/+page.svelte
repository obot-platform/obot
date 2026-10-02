<script lang="ts">
	import TabLayout from '$lib/components/TabLayout.svelte';
	import Devices from '$lib/components/admin/devices/Devices.svelte';
	import { m } from '$lib/i18n';
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
					{
						label: m.routes_inv_tab_overview(),
						value: 'overview',
						content: overview,
						tooltip: m.routes_inv_tab_overview_tooltip()
					},
					{
						label: m.routes_inv_tab_configuration(),
						value: 'configuration',
						content: configuration,
						tooltip: m.routes_inv_tab_configuration_tooltip()
					}
				]
			: []),
		{
			label: m.routes_inv_tab_devices(),
			value: 'devices',
			content: devices,
			tooltip: m.routes_inv_tab_devices_tooltip()
		},
		...(profile.current.hasAdminAccess?.()
			? [
					{
						label: m.routes_inv_tab_device_clients(),
						value: 'device-clients',
						content: deviceClients,
						tooltip: m.routes_inv_tab_device_clients_tooltip()
					},
					{
						label: m.routes_inv_tab_device_mcp_servers(),
						value: 'device-mcp-servers',
						content: deviceMcpServers,
						tooltip: m.routes_inv_tab_device_mcp_servers_tooltip()
					},
					{
						label: m.routes_inv_tab_device_skills(),
						value: 'device-skills',
						content: deviceSkills,
						tooltip: m.routes_inv_tab_device_skills_tooltip()
					}
				]
			: [])
	]);
</script>

<svelte:head>
	<title>{m.routes_inv_page_title()}</title>
</svelte:head>

<TabLayout
	title={m.nav_inventory()}
	{defaultView}
	classes={{ childrenContainer: 'max-w-none' }}
	{views}
/>

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
