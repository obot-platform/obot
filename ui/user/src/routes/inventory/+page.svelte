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
						label: m.inventory_enforcement_overview_tab(),
						value: 'overview',
						content: overview,
						tooltip: m.inventory_enforcement_overview_tab_tooltip()
					},
					{
						label: m.inventory_enforcement_configuration_tab(),
						value: 'configuration',
						content: configuration,
						tooltip: m.inventory_enforcement_configuration_tab_tooltip()
					}
				]
			: []),
		{
			label: m.inventory_enforcement_devices_tab(),
			value: 'devices',
			content: devices,
			tooltip: m.inventory_enforcement_devices_tab_tooltip()
		},
		...(profile.current.hasAdminAccess?.()
			? [
					{
						label: m.inventory_enforcement_device_clients_tab(),
						value: 'device-clients',
						content: deviceClients,
						tooltip: m.inventory_enforcement_device_clients_tab_tooltip()
					},
					{
						label: m.inventory_enforcement_device_mcp_servers_tab(),
						value: 'device-mcp-servers',
						content: deviceMcpServers,
						tooltip: m.inventory_enforcement_device_mcp_servers_tab_tooltip()
					},
					{
						label: m.inventory_enforcement_device_skills_tab(),
						value: 'device-skills',
						content: deviceSkills,
						tooltip: m.inventory_enforcement_device_skills_tab_tooltip()
					}
				]
			: [])
	]);
</script>

<svelte:head>
	<title>{m.inventory_enforcement_page_title()}</title>
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
