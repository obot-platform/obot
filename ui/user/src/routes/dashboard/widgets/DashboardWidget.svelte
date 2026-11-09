<script lang="ts">
	import Skeleton from '$lib/components/Skeleton.svelte';
	import DeviceScanDonutCard from '$lib/components/admin/device-scan/DeviceScanDonutCard.svelte';
	import DeviceScanTimelineCard from '$lib/components/admin/device-scan/DeviceScanTimelineCard.svelte';
	import TokenUsageOverallStats from '$lib/components/admin/token-usage/TokenUsageOverallStats.svelte';
	import TokenUsageRequestTiles from '$lib/components/admin/token-usage/TokenUsageRequestTiles.svelte';
	import TokenUsageRequestsChart from '$lib/components/admin/token-usage/TokenUsageRequestsChart.svelte';
	import TokenUsageSpendByProvider from '$lib/components/admin/token-usage/TokenUsageSpendByProvider.svelte';
	import TokenUsageTimelineCard from '$lib/components/admin/token-usage/TokenUsageTimelineCard.svelte';
	import TokenUsageTopModels from '$lib/components/admin/token-usage/TokenUsageTopModels.svelte';
	import { usageUserDisplayName } from '$lib/components/admin/usage/utils';
	import GraphTitle from '$lib/components/graph/GraphTitle.svelte';
	import { m } from '$lib/i18n';
	import { withDisabledMarker } from '$lib/utils';
	import type { WidgetId } from '../dashboardLayout';
	import DeviceScanStats from './DeviceScanStats.svelte';
	import MostDeployedServers from './MostDeployedServers.svelte';
	import PlatformStats from './PlatformStats.svelte';
	import PopularTools from './PopularTools.svelte';
	import ServerActivity from './ServerActivity.svelte';
	import ToolResponseTime from './ToolResponseTime.svelte';
	import TopServersUsed from './TopServersUsed.svelte';
	import UsageBarCard from './UsageBarCard.svelte';
	import type { DashboardWidgetModel } from './model';
	import { fade } from 'svelte/transition';

	type Props = {
		id: WidgetId;
		model: DashboardWidgetModel;
	};

	let { id, model }: Props = $props();

	const requestStats = $derived(model.loadingTokenUsageStats ? undefined : model.tokenUsageStats);

	function toolNameLabel(value: string | number) {
		const label = String(value).split('.').slice(1).join('.');
		return label || String(value);
	}

	function toolNameTail(value: string | number) {
		const parts = String(value).split('.');
		return parts[parts.length - 1] || String(value);
	}

	function activeUserLabel(userId: string) {
		const user = model.users.find((entry) => entry.id === userId);
		return withDisabledMarker(usageUserDisplayName(user), user);
	}
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
{:else if id === 'model-stats'}
	<TokenUsageOverallStats
		title={m.dashboard_model_stats()}
		loading={model.loadingTokens}
		totals={model.totalTokenUsage}
		visible={model.modelStats}
		class="h-full"
	/>
{:else if id === 'llm-requests'}
	<div class="@container paper flex h-full min-h-72 flex-col gap-0 overflow-hidden p-0">
		<GraphTitle title={m.audit_usage_usage_requests_over_time()} element="h4" />
		<TokenUsageRequestTiles
			framed={false}
			loading={model.loadingTokenUsageStats}
			stats={requestStats}
		/>
		<div class="w-full shrink-0 p-4">
			<TokenUsageRequestsChart
				framed={false}
				loading={model.loadingTokenUsageStats}
				stats={requestStats}
				startDate={model.start}
				endDate={model.end}
			/>
		</div>
	</div>
{:else if id === 'spend-by-provider'}
	<TokenUsageSpendByProvider
		loading={model.loadingTokenUsageStats}
		stats={requestStats}
		class="h-full"
	/>
{:else if id === 'top-models'}
	<TokenUsageTopModels
		loading={model.loadingTokenUsageStats}
		stats={requestStats}
		title={m.dashboard_top_models()}
		class="h-full"
	/>
{:else if id === 'most-frequent-tool-calls'}
	<UsageBarCard
		loading={model.loadingToolUsage}
		title={m.audit_usage_usage_most_frequent_tool_calls()}
		data={model.topToolCalls}
		labelKey="toolLabel"
		valueKey="count"
		formatLabel={toolNameLabel}
		formatTooltipText={(row) =>
			m.audit_usage_usage_tooltip_calls({ count: row.count, name: row.serverDisplayName })}
	/>
{:else if id === 'tool-individual-response-time'}
	<UsageBarCard
		loading={model.loadingToolUsage}
		title={m.audit_usage_usage_individual_response_time()}
		data={model.individualToolResponses}
		labelKey="toolName"
		valueKey="processingTimeMs"
		formatLabel={toolNameTail}
		formatTooltipText={(row) =>
			`${m.audit_usage_usage_latency_ms({ ms: row.processingTimeMs.toFixed(2) })} • ${row.serverDisplayName}`}
	/>
{:else if id === 'tool-call-errors'}
	<UsageBarCard
		loading={model.loadingToolUsage}
		title={m.audit_usage_usage_tool_call_errors()}
		data={model.toolCallErrors}
		labelKey="toolName"
		valueKey="errorCount"
		formatLabel={toolNameTail}
		formatTooltipText={(row) =>
			m.audit_usage_usage_tooltip_errors({ count: row.errorCount, name: row.serverDisplayName })}
	/>
{:else if id === 'tool-call-errors-by-server'}
	<UsageBarCard
		loading={model.loadingToolUsage}
		title={m.audit_usage_usage_errors_by_server()}
		data={model.toolCallErrorsByServer}
		labelKey="serverName"
		valueKey="errorCount"
		formatTooltipText={(row) => `${row.errorCount} ${m.audit_usage_usage_unit_errors()}`}
	/>
{:else if id === 'most-active-users'}
	<UsageBarCard
		loading={model.loadingToolUsage}
		title={m.audit_usage_usage_most_active_users()}
		data={model.mostActiveUsers}
		labelKey="userId"
		valueKey="callCount"
		formatLabel={(userId) => activeUserLabel(String(userId))}
		formatTooltipText={(row) =>
			m.audit_usage_usage_tooltip_calls({
				count: row.callCount,
				name: activeUserLabel(row.userId)
			})}
	/>
{/if}
