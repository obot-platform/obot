<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { NanobotService } from '$lib/services';
	import type { OrgUser } from '$lib/services/admin/types';
	import { getMcpServerDeploymentStatus } from '$lib/services/chat/mcp';
	import { profile, version } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { goto } from '$lib/url';
	import { getUserDisplayName, openUrl } from '$lib/utils';
	import { HatGlasses } from 'lucide-svelte';
	import { untrack } from 'svelte';

	let { data } = $props();
	let query = $derived(page.url.searchParams.get('query') || '');

	let agents = $state(untrack(() => data.agents));
	let users = $state<OrgUser[]>(untrack(() => data.users));
	let launchingAgentId = $state<string | null>(null);
	let confirmImpersonate = $state<{ userDisplayName: string; agent: TableItem } | null>(null);

	const doesSupportK8sUpdates = $derived(version.current.engine === 'kubernetes');
	const userMap = $derived(new Map(users.map((u) => [u.id, u])));
	const tableData = $derived(
		agents
			.map((agent) => {
				const { updateStatus, updatesAvailable, updateStatusTooltip } =
					getMcpServerDeploymentStatus(agent, doesSupportK8sUpdates);
				return {
					...agent,
					ownerDisplay: getUserDisplayName(userMap, agent.userID),
					isMyServer: agent.userID === profile.current?.id,
					updateStatus,
					updatesAvailable,
					updateStatusTooltip
				};
			})
			.filter((agent) => agent.ownerDisplay.toLowerCase().includes(query.toLowerCase()))
	);

	type TableItem = (typeof tableData)[0];

	async function impersonate(agent?: TableItem) {
		if (!agent) return;
		launchingAgentId = agent.id;
		try {
			await NanobotService.launchProjectV2Agent(agent.projectV2ID, agent.id);
			window.open(`/agent?projectId=${agent.projectV2ID}&agentId=${agent.id}`, '_blank');
		} catch (error) {
			console.error('Failed to launch agent:', error);
		} finally {
			launchingAgentId = null;
			confirmImpersonate = null;
		}
	}
</script>

<Layout title="Agents">
	<div class="flex flex-col gap-4">
		<div class="flex items-center justify-between">
			<p class="text-sm text-gray-500">
				Browse and connect to agents across all users. Clicking "Connect" will open the agent's chat
				interface in a new tab.
			</p>
		</div>

		<Search
			value={query}
			class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
			onChange={(v) => {
				const currentUrl = new URL(page.url);
				if (v) {
					currentUrl.searchParams.set('query', v);
				} else {
					currentUrl.searchParams.delete('query');
				}
				goto(currentUrl, { replaceState: true, keepFocus: true });
			}}
			placeholder="Search by owner..."
		/>

		<Table
			data={tableData}
			fields={[
				'ownerDisplay',
				...(doesSupportK8sUpdates ? ['deploymentStatus'] : []),
				'updatesAvailable',
				'created'
			]}
			filterable={['ownerDisplay', 'deploymentStatus', 'updatesAvailable']}
			headers={[
				{ title: 'Owner', property: 'ownerDisplay' },
				{ title: 'Health', property: 'deploymentStatus' },
				{ title: 'Update Status', property: 'updatesAvailable' }
			]}
			sortable={['ownerDisplay', 'deploymentStatus', 'updatesAvailable', 'created']}
			noDataMessage="No agents found."
			onClickRow={(agent, isCtrlClick) => {
				openUrl(`/admin/agents/p/${agent.projectV2ID}/s/${agent.id}/details`, isCtrlClick);
			}}
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'created'}
					{formatTimeAgo(d.created).relativeTime}
				{:else if property === 'updatesAvailable'}
					<div
						use:tooltip={{ text: d.updateStatusTooltip ?? '', classes: ['whitespace-pre-line'] }}
					>
						{d.updateStatus || '--'}
					</div>
				{:else if property === 'deploymentStatus'}
					{d.deploymentStatus || '--'}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(agent)}
				<button
					class="icon-button hover:text-primary"
					onclick={(e) => {
						e.stopPropagation();
						confirmImpersonate = { userDisplayName: agent.ownerDisplay, agent };
					}}
					disabled={launchingAgentId === agent.id ||
						!profile.current.canImpersonate?.() ||
						agent.userID === profile.current.id}
					use:tooltip={profile.current.canImpersonate?.() && agent.userID !== profile.current.id
						? `Impersonate ${agent.ownerDisplay}`
						: agent.userID === profile.current.id
							? 'You cannot impersonate yourself.'
							: 'You do not have permission to impersonate other users.'}
				>
					<HatGlasses class="size-4" />
				</button>
			{/snippet}
		</Table>
	</div>
</Layout>

<Confirm
	show={Boolean(confirmImpersonate)}
	oncancel={() => (confirmImpersonate = null)}
	onsuccess={() => impersonate(confirmImpersonate?.agent)}
	type="info"
	title="Confirm Agent Connection"
	msg={`Connect as ${confirmImpersonate?.userDisplayName || 'user'}?`}
	loading={Boolean(launchingAgentId)}
>
	{#snippet note()}
		<p>
			This will allow you to connect to the agent, impersonating as <b class="font-semibold"
				>{confirmImpersonate?.userDisplayName || 'user'}</b
			>. Any actions you take will be attributed to this user. Are you sure you wish to continue?
		</p>
		<p class="text-on-surface1 mt-4 text-sm">Note: This will open in a new window.</p>
	{/snippet}
</Confirm>

<svelte:head>
	<title>Obot | Agents</title>
</svelte:head>
