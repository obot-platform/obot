<script lang="ts">
	import { invalidate } from '$app/navigation';
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import AccessControlRuleForm from '$lib/components/admin/AccessControlRuleForm.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import {
		MCP_ACCESS_POLICY_FIELD_IDS,
		MCP_PUBLISHER_ALL_OPTION,
		PAGE_TRANSITION_DURATION
	} from '$lib/constants';
	import {
		fetchMcpServerAndEntries,
		getPoweruserWorkspace,
		initMcpServerAndEntries
	} from '$lib/context/poweruserWorkspace.svelte';
	import { AdminService, UserService, type OrgUser, type AccessControlRule } from '$lib/services';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import { goto, clearUrlParams } from '$lib/url';
	import { getUserDisplayName, openUrl } from '$lib/utils';
	import { BookOpenText, Plus, Trash2 } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';
	import { fade, fly } from 'svelte/transition';

	interface Props {
		accessControlRules: AccessControlRule[];
		creating?: boolean;
		workspaceId?: string;
	}

	let { accessControlRules: initialRules, creating = false, workspaceId }: Props = $props();

	let accessControlRules = $state<AccessControlRule[]>(untrack(() => initialRules));
	$effect(() => {
		accessControlRules = initialRules;
	});

	let ruleToDelete = $state<AccessControlRule>();
	let users = $state<OrgUser[]>([]);
	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));
	let isAdmin = $derived(profile.current.hasAdminAccess?.());
	let isReadonly = $derived(profile.current.isAdminReadonly?.());

	if (!untrack(() => profile.current.hasAdminAccess?.())) {
		initMcpServerAndEntries();
	}

	const poweruserWorkspace = untrack(() =>
		profile.current.hasAdminAccess?.() ? undefined : getPoweruserWorkspace()
	);
	const totalWorkspaceServers = $derived(
		(poweruserWorkspace?.entries.length ?? 0) + (poweruserWorkspace?.servers.length ?? 0)
	);

	let validAccessControlRules = $derived(
		accessControlRules.filter((rule) => (rule.powerUserID ? usersMap.has(rule.powerUserID) : true))
	);

	function convertToTableData(rule: AccessControlRule, registry: 'user' | 'global' = 'global') {
		const owner = rule.powerUserID ? getUserDisplayName(usersMap, rule.powerUserID) : undefined;
		const totalServers =
			mcpServersAndEntries.current.entries.length + mcpServersAndEntries.current.servers.length;

		const hasEverything = rule.resources?.find((r) => r.id === '*');
		const count = (() => {
			if (registry === 'global') {
				if (hasEverything) return totalServers;

				return (
					(rule.resources &&
						rule.resources.filter(
							(r) => r.type === 'mcpServerCatalogEntry' || r.type === 'mcpServer'
						).length) ??
					0
				);
			}

			if (hasEverything) return getAcrServerCount(rule.powerUserWorkspaceID!);

			return (
				(rule.resources &&
					rule.resources.filter((r) => r.type === 'mcpServerCatalogEntry' || r.type === 'mcpServer')
						.length) ??
				0
			);
		})();

		return {
			...rule,
			owner: owner || 'Unknown',
			serversCount: count || 0
		};
	}
	let globalAccessControlRules = $derived(
		validAccessControlRules.filter((rule) => !rule.powerUserID).map((d) => convertToTableData(d))
	);
	let userAccessControlRules = $derived(
		validAccessControlRules
			.filter((rule) => rule.powerUserID)
			.map((d) => convertToTableData(d, 'user'))
	);

	async function navigateToCreated(rule: AccessControlRule) {
		clearUrlParams(['new']);
		goto(`/mcp-servers/access-policies/${rule.id}`);
	}

	function getAcrServerCount(powerUserWorkspaceID: string) {
		const mcpServers = Array.from(mcpServersAndEntries.current.servers.values());
		const mcpEntries = Array.from(mcpServersAndEntries.current.entries.values());

		return (
			mcpServers.filter((server) => server.powerUserWorkspaceID === powerUserWorkspaceID).length +
			mcpEntries.filter((entry) => entry.powerUserWorkspaceID === powerUserWorkspaceID).length
		);
	}

	function policyDetailUrl(rule: AccessControlRule) {
		if (isAdmin && rule.powerUserWorkspaceID) {
			return `/mcp-servers/access-policies/w/${rule.powerUserWorkspaceID}/r/${rule.id}`;
		}
		return `/mcp-servers/access-policies/${rule.id}`;
	}

	onMount(async () => {
		if (isAdmin) {
			users = await UserService.listUsersIncludeDeleted();
			return;
		}
		if (workspaceId) {
			fetchMcpServerAndEntries(workspaceId);
		}
	});

	const duration = PAGE_TRANSITION_DURATION;
</script>

{#if creating}
	{@render createRuleScreen()}
{:else}
	<div class="flex flex-col gap-8" in:fade={{ duration }}>
		{#if accessControlRules.length === 0}
			<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<BookOpenText class="text-muted-content size-24 opacity-25" />
				<h4 class="text-muted-content text-lg font-semibold">No created MCP access policies</h4>
				<p class="text-muted-content text-sm font-light">
					Looks like you don't have any access policies created yet. <br />
					{#if !isReadonly}
						Click the button below to get started.
					{/if}
				</p>

				{@render addRuleButton(MCP_ACCESS_POLICY_FIELD_IDS.addPolicyEmptyBtn)}
			</div>
		{:else if isAdmin}
			<div class="flex flex-col gap-2">
				<h4 class="text-base font-semibold">Admin Managed Access Policies</h4>
				{@render accessControlRuleTable('global')}
			</div>

			<details class="collapse bg-base-300 collapse-arrow mb-2 w-full border border-transparent">
				<summary class="collapse-title text-base font-semibold"
					>User Managed Access Policies</summary
				>
				<div class="collapse-content bg-base-200 p-2 text-sm">
					{@render accessControlRuleTable('user')}
				</div>
			</details>
		{:else}
			<Table
				data={accessControlRules}
				fields={['displayName', 'servers']}
				onClickRow={(d, isCtrlClick) => {
					openUrl(policyDetailUrl(d), isCtrlClick);
				}}
				headers={[
					{
						title: 'Name',
						property: 'displayName'
					}
				]}
			>
				{#snippet actions(d)}
					{#if !isReadonly}
						<IconButton
							variant="danger"
							onclick={(e) => {
								e.stopPropagation();
								ruleToDelete = d;
							}}
							tooltip={{ text: 'Delete Rule' }}
						>
							<Trash2 class="size-4" />
						</IconButton>
					{/if}
				{/snippet}
				{#snippet onRenderColumn(property, d)}
					{#if property === 'servers'}
						{@const hasEverything = d.resources?.find((r) => r.id === '*')}
						{@const count = hasEverything
							? totalWorkspaceServers
							: ((d.resources &&
									d.resources.filter(
										(r) => r.type === 'mcpServerCatalogEntry' || r.type === 'mcpServer'
									).length) ??
								0)}
						{count ? count : '-'}
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
			</Table>
		{/if}
	</div>
{/if}

{#snippet accessControlRuleTable(type: 'user' | 'global')}
	{@const data = type === 'global' ? globalAccessControlRules : userAccessControlRules}
	<Table
		{data}
		fields={type === 'global'
			? ['displayName', 'serversCount']
			: ['displayName', 'serversCount', 'owner']}
		onClickRow={(d, isCtrlClick) => {
			openUrl(policyDetailUrl(d), isCtrlClick);
		}}
		headers={[
			{
				title: 'Name',
				property: 'displayName'
			},
			{
				title: 'Servers',
				property: 'serversCount'
			}
		]}
		filterable={['displayName', 'owner']}
		sortable={['displayName', 'serversCount', 'owner']}
	>
		{#snippet actions(d)}
			{#if !isReadonly}
				<IconButton
					variant="danger"
					onclick={(e) => {
						e.stopPropagation();
						ruleToDelete = d;
					}}
					tooltip={{ text: 'Delete Rule' }}
				>
					<Trash2 class="size-4" />
				</IconButton>
			{/if}
		{/snippet}
		{#snippet onRenderColumn(property, d)}
			{#if property === 'serversCount'}
				{d.serversCount === 0 ? '-' : d.serversCount}
			{:else}
				{d[property as keyof typeof d]}
			{/if}
		{/snippet}
	</Table>
{/snippet}

{#snippet addRuleButton(id: string)}
	{#if !profile.current.isAdminReadonly?.()}
		<button
			{id}
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => {
				goto(
					`${page.url.pathname}?view=${page.url.searchParams.get('view') ?? 'access-policies'}&new=true`
				);
			}}
		>
			<Plus class="size-4" /> Add Access Policy
		</button>
	{/if}
{/snippet}

{#snippet createRuleScreen()}
	<div class="h-full w-full" in:fly|global={{ x: 100, delay: duration, duration }}>
		{#if isAdmin}
			<AccessControlRuleForm
				onCreate={navigateToCreated}
				mcpEntriesContextFn={() => mcpServersAndEntries.current}
			/>
		{:else}
			<AccessControlRuleForm
				onCreate={navigateToCreated}
				entity="workspace"
				id={workspaceId}
				mcpEntriesContextFn={getPoweruserWorkspace}
				all={MCP_PUBLISHER_ALL_OPTION}
			/>
		{/if}
	</div>
{/snippet}

<Confirm
	msg={`Delete ${ruleToDelete?.displayName || 'this rule'}?`}
	show={Boolean(ruleToDelete)}
	onsuccess={async () => {
		if (!ruleToDelete) return;
		if (!isAdmin && workspaceId) {
			await UserService.deleteWorkspaceAccessControlRule(workspaceId, ruleToDelete.id);
			accessControlRules = await UserService.listWorkspaceAccessControlRules(workspaceId);
		} else if (ruleToDelete.powerUserWorkspaceID) {
			await UserService.deleteWorkspaceAccessControlRule(
				ruleToDelete.powerUserWorkspaceID,
				ruleToDelete.id
			);
		} else {
			await AdminService.deleteAccessControlRule(ruleToDelete.id);
		}

		invalidate('mcp-access-policies:data');

		ruleToDelete = undefined;
	}}
	oncancel={() => (ruleToDelete = undefined)}
/>
