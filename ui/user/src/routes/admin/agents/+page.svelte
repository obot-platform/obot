<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { m } from '$lib/i18n';
	import { NanobotService, type OrgUser } from '$lib/services';
	import {
		getMcpServerDeploymentStatus,
		getMcpValueLabel,
		mcpTableDisplayValue
	} from '$lib/services/user/mcp';
	import { profile, version } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { goto } from '$lib/url';
	import { getUserDisplayName, openUrl } from '$lib/utils';
	import { HatGlasses } from '@lucide/svelte';
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
			await NanobotService.launchProjectAgent(agent.projectID, agent.id);
			window.open(`/agent?projectId=${agent.projectID}&agentId=${agent.id}`, '_blank');
		} catch (error) {
			console.error('Failed to launch agent:', error);
		} finally {
			launchingAgentId = null;
			confirmImpersonate = null;
		}
	}
</script>

<Layout title={m.identity_access_agents_tab()}>
	<div class="flex flex-col gap-4">
		<div class="flex items-center justify-between">
			<p class="text-sm text-muted-content">
				{m.identity_access_agents_browse_description()}
			</p>
		</div>

		<Search
			value={query}
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
			onChange={(v) => {
				const currentUrl = new URL(page.url);
				if (v) {
					currentUrl.searchParams.set('query', v);
				} else {
					currentUrl.searchParams.delete('query');
				}
				goto(currentUrl, { replaceState: true, keepFocus: true });
			}}
			placeholder={m.identity_access_agents_search_owner()}
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
				{ title: m.core_role_owner(), property: 'ownerDisplay' },
				{ title: m.identity_access_agents_col_health(), property: 'deploymentStatus' },
				{ title: m.core_col_update_status(), property: 'updatesAvailable' },
				{ title: m.core_col_created(), property: 'created' }
			]}
			sortable={['ownerDisplay', 'deploymentStatus', 'updatesAvailable', 'created']}
			displayValue={mcpTableDisplayValue}
			noDataMessage={m.identity_access_agents_none_found()}
			onClickRow={(agent, isCtrlClick) => {
				openUrl(`/admin/agents/p/${agent.projectID}/s/${agent.id}/details`, isCtrlClick);
			}}
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'created'}
					{formatTimeAgo(d.created).relativeTime}
				{:else if property === 'updatesAvailable'}
					<div
						use:tooltip={{ text: d.updateStatusTooltip ?? '', classes: ['whitespace-pre-line'] }}
					>
						{getMcpValueLabel(d.updateStatus) || '--'}
					</div>
				{:else if property === 'deploymentStatus'}
					{getMcpValueLabel(d.deploymentStatus) || '--'}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(agent)}
				<IconButton
					variant="primary"
					onclick={(e) => {
						e.stopPropagation();
						confirmImpersonate = { userDisplayName: agent.ownerDisplay, agent };
					}}
					disabled={launchingAgentId === agent.id ||
						!profile.current.canImpersonate?.() ||
						agent.userID === profile.current.id}
					tooltip={{
						text:
							profile.current.canImpersonate?.() && agent.userID !== profile.current.id
								? m.identity_access_agents_impersonate_named({ name: agent.ownerDisplay })
								: agent.userID === profile.current.id
									? m.identity_access_agents_cannot_impersonate_self()
									: m.identity_access_agents_no_impersonate_permission()
					}}
				>
					<HatGlasses class="size-4" />
				</IconButton>
			{/snippet}
		</Table>
	</div>
</Layout>

<Confirm
	show={Boolean(confirmImpersonate)}
	oncancel={() => (confirmImpersonate = null)}
	onsuccess={() => impersonate(confirmImpersonate?.agent)}
	type="info"
	title={m.identity_access_agents_confirm_connection()}
	msg={m.identity_access_agents_connect_as({
		name: confirmImpersonate?.userDisplayName || m.identity_access_agents_user_fallback()
	})}
	loading={Boolean(launchingAgentId)}
>
	{#snippet note()}
		<p>
			{m.identity_access_agents_impersonate_note_prefix()}
			<b class="font-semibold"
				>{confirmImpersonate?.userDisplayName || m.identity_access_agents_user_fallback()}</b
			>{m.identity_access_agents_impersonate_note_suffix()}
		</p>
		<p class="text-muted-content mt-4 text-sm">{m.identity_access_agents_new_window_note()}</p>
	{/snippet}
</Confirm>

<svelte:head>
	<title>Obot | {m.identity_access_agents_tab()}</title>
</svelte:head>
