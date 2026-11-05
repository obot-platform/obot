<script lang="ts">
	import Skeleton from '$lib/components/Skeleton.svelte';
	import DeviceScanDonutCard from '$lib/components/admin/device-scan/DeviceScanDonutCard.svelte';
	import DeviceScanTimelineCard from '$lib/components/admin/device-scan/DeviceScanTimelineCard.svelte';
	import TokenUsageTimelineCard from '$lib/components/admin/token-usage/TokenUsageTimelineCard.svelte';
	import { m } from '$lib/i18n';
	import type { WidgetId } from '../dashboardLayout';
	import DeviceScanStats from './DeviceScanStats.svelte';
	import MostDeployedServers from './MostDeployedServers.svelte';
	import PlatformStats from './PlatformStats.svelte';
	import PopularTools from './PopularTools.svelte';
	import ServerActivity from './ServerActivity.svelte';
	import ToolResponseTime from './ToolResponseTime.svelte';
	import TopServersUsed from './TopServersUsed.svelte';
	import type { DashboardWidgetModel } from './model';
	import { fade } from 'svelte/transition';

	type Props = {
		id: WidgetId;
		model: DashboardWidgetModel;
	};

	let { id, model }: Props = $props();
</script>

{#if id === 'platform-stats'}
	<PlatformStats tiles={model.platformTiles} isBootStrapUser={model.isBootStrapUser} />
{:else if id === 'device-scan-stats'}
	<DeviceScanStats tiles={model.deviceTiles} />
{:else if id === 'server-activity'}
	<ServerActivity
		loading={model.serverLoading}
		hasDeviceScans={model.hasDeviceScans}
		doesSupportK8sUpdates={model.doesSupportK8sUpdates}
		deploymentStatusBreakdown={model.deploymentStatusBreakdown}
		totalServers={model.totalServers}
		graphData={model.graphData}
		isBootStrapUser={model.isBootStrapUser}
	/>
{:else if id === 'top-device-skills'}
	{#if model.loadingDeviceScanStats}
		<Skeleton type="card" class="h-full min-h-72 w-full" />
	{:else}
		<div class="h-full min-h-0" in:fade={{ duration: 150 }}>
			<DeviceScanDonutCard
				title={m.dashboard_top_device_skills()}
				buckets={model.deviceScanSkillBuckets}
				totalGroups={model.totalDeviceScanSkillGroups}
				emptyMsg={m.dashboard_no_skills_observed()}
				class="h-full"
				classes={{ graphContainer: '@md:w-1/2', graph: 'h-56 w-full' }}
			/>
		</div>
	{/if}
{:else if id === 'most-deployed'}
	<MostDeployedServers
		loading={model.serversLoading}
		servers={model.popularServers}
		isBootStrapUser={model.isBootStrapUser}
	/>
{:else if id === 'device-clients'}
	{#if model.loadingDeviceScanStats}
		<Skeleton type="card" class="h-full min-h-72 w-full" />
	{:else}
		<div class="h-full min-h-0" in:fade={{ duration: 150 }}>
			<DeviceScanDonutCard
				legendOnBottom
				title={m.dashboard_device_clients()}
				buckets={model.deviceScanClientBuckets}
				totalGroups={model.totalDeviceScanClientGroups}
				emptyMsg={m.dashboard_no_clients_observed()}
				class="h-full"
			/>
		</div>
	{/if}
{:else if id === 'top-device-mcps'}
	{#if model.loadingDeviceScanStats}
		<Skeleton type="card" class="h-full min-h-72 w-full" />
	{:else}
		<div class="h-full min-h-0" in:fade={{ duration: 150 }}>
			<DeviceScanDonutCard
				legendOnBottom
				title={m.dashboard_top_device_mcp_servers()}
				buckets={model.deviceScanMcpBuckets}
				totalGroups={model.totalDeviceScanMcpGroups}
				emptyMsg={m.dashboard_no_mcp_servers_observed()}
				class="h-full"
			/>
		</div>
	{/if}
{:else if id === 'top-servers-used'}
	<TopServersUsed
		loading={model.loadingToolUsage}
		rows={model.topServerUsage}
		maxServersToShow={model.maxServersToShow}
	/>
{:else if id === 'device-scan-timeline'}
	{#if model.loadingDeviceScanStats}
		<Skeleton type="card" class="h-full min-h-80 w-full" />
	{:else}
		<div class="h-full min-h-0" in:fade={{ duration: 150 }}>
			<DeviceScanTimelineCard
				rangeStart={model.start}
				rangeEnd={model.end}
				timelineRows={model.deviceScanTimelineRows}
				totalSubmissions={model.totalDeviceScanSubmissions}
			/>
		</div>
	{/if}
{:else if id === 'token-usage'}
	<TokenUsageTimelineCard startDate={model.start} endDate={model.end} class="h-full" />
{:else if id === 'popular-tools'}
	<PopularTools
		loading={model.loadingToolUsage}
		rows={model.topToolCalls}
		maxToolsToShow={model.maxToolsToShow}
		isBootStrapUser={model.isBootStrapUser}
	/>
{:else if id === 'tool-response-time'}
	<ToolResponseTime
		loading={model.loadingToolUsage}
		rows={model.avgToolCallResponseTime}
		maxToolsToShow={model.maxToolsToShow}
		isBootStrapUser={model.isBootStrapUser}
	/>
{/if}
