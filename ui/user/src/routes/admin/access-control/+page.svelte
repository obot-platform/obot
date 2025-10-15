<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { BookOpenText, ChevronLeft, Plus, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { type AccessControlRule, type OrgUser } from '$lib/services/admin/types';
	import Confirm from '$lib/components/Confirm.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import AccessControlRuleForm from '$lib/components/admin/AccessControlRuleForm.svelte';
	import { onMount } from 'svelte';
	import {
		fetchMcpServerAndEntries,
		getAdminMcpServerAndEntries,
		initMcpServerAndEntries
	} from '$lib/context/admin/mcpServerAndEntries.svelte';
	import { AdminService, ChatService } from '$lib/services/index.js';
	import { getUserDisplayName, openUrl } from '$lib/utils.js';
	import { profile } from '$lib/stores/index.js';

	let { data } = $props();
	const { accessControlRules: initialRules } = data;
	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;

	initMcpServerAndEntries();

	const mcpServersAndEntries = getAdminMcpServerAndEntries();
	let accessControlRules = $state(initialRules);
	let showCreateRule = $state(false);
	let ruleToDelete = $state<AccessControlRule>();

	let users = $state<OrgUser[]>([]);
	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));

	let validAccessControlRules = $derived(
		accessControlRules.filter((rule) => (rule.powerUserID ? usersMap.has(rule.powerUserID) : true))
	);

	function convertToTableData(rule: AccessControlRule, registry: 'user' | 'global' = 'global') {
		const owner = rule.powerUserID ? getUserDisplayName(usersMap, rule.powerUserID) : undefined;
		const totalServers = mcpServersAndEntries.entries.length + mcpServersAndEntries.servers.length;

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

	let isReadonly = $derived(profile.current.isAdminReadonly?.());

	onMount(() => {
		const url = new URL(window.location.href);
		const queryParams = new URLSearchParams(url.search);
		if (queryParams.get('new')) {
			showCreateRule = true;
		}
	});

	async function navigateToCreated(rule: AccessControlRule) {
		showCreateRule = false;
		goto(`/admin/access-control/${rule.id}`, { replaceState: false });
	}

	const duration = PAGE_TRANSITION_DURATION;

	onMount(async () => {
		fetchMcpServerAndEntries(defaultCatalogId);
		users = await AdminService.listUsersIncludeDeleted();
	});

	function getAcrServerCount(powerUserWorkspaceID: string) {
		const mcpServers = Array.from(mcpServersAndEntries.servers.values());
		return mcpServers.filter((server) => server.powerUserWorkspaceID === powerUserWorkspaceID)
			.length;
	}
</script>

<Layout>
	<div
		class="my-4 h-full w-full"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if showCreateRule}
			{@render createRuleScreen()}
		{:else}
			<div
				class="flex flex-col gap-8"
				in:fly={{ x: 100, delay: duration, duration }}
				out:fly={{ x: -100, duration }}
			>
				<div class="flex items-center justify-between">
					<h1 class="text-2xl font-semibold">Access Control</h1>
					{#if accessControlRules.length > 0}
						<div class="relative flex items-center gap-4">
							{@render addRuleButton()}
						</div>
					{/if}
				</div>
				{#if accessControlRules.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<BookOpenText class="size-24 text-gray-200 dark:text-gray-900" />
						<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
							No created access control rules
						</h4>
						<p class="text-sm font-light text-gray-400 dark:text-gray-600">
							Looks like you don't have any rules created yet. <br />
							{#if !isReadonly}
								Click the button below to get started.
							{/if}
						</p>

						{@render addRuleButton()}
					</div>
				{:else}
					<div class="flex flex-col gap-2">
						<h2 class="text-xl font-semibold">Global Access Control Rules</h2>
						{@render accessControlRuleTable('global')}
					</div>

					<div class="flex flex-col gap-2">
						<h2 class="text-xl font-semibold">User Created Access Control Rules</h2>
						{@render accessControlRuleTable('user')}
					</div>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

{#snippet accessControlRuleTable(type: 'global' | 'user')}
	{@const data = type === 'user' ? userAccessControlRules : globalAccessControlRules}
	<Table
		{data}
		fields={type === 'user'
			? ['displayName', 'serversCount', 'owner']
			: ['displayName', 'serversCount']}
		onClickRow={(d, isCtrlClick) => {
			const url = d.powerUserWorkspaceID
				? `/admin/access-control/w/${d.powerUserWorkspaceID}/r/${d.id}`
				: `/admin/access-control/${d.id}`;
			openUrl(url, isCtrlClick);
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
				<button
					class="icon-button hover:text-red-500"
					onclick={(e) => {
						e.stopPropagation();
						ruleToDelete = d;
					}}
					use:tooltip={'Delete Rule'}
				>
					<Trash2 class="size-4" />
				</button>
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

{#snippet addRuleButton()}
	{#if !profile.current.isAdminReadonly?.()}
		<button
			class="button-primary flex items-center gap-1 text-sm"
			onclick={() => (showCreateRule = true)}
		>
			<Plus class="size-4" /> Add New Rule
		</button>
	{/if}
{/snippet}

{#snippet createRuleScreen()}
	<div
		class="h-full w-full"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<AccessControlRuleForm
			onCreate={navigateToCreated}
			mcpEntriesContextFn={getAdminMcpServerAndEntries}
		>
			{#snippet topContent()}
				<button
					onclick={() => (showCreateRule = false)}
					class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
				>
					<ChevronLeft class="size-6" />
					Access Control
				</button>
			{/snippet}
		</AccessControlRuleForm>
	</div>
{/snippet}

<Confirm
	msg="Are you sure you want to delete this rule?"
	show={Boolean(ruleToDelete)}
	onsuccess={async () => {
		if (!ruleToDelete) return;
		if (ruleToDelete.powerUserWorkspaceID) {
			await ChatService.deleteWorkspaceAccessControlRule(
				ruleToDelete.powerUserWorkspaceID,
				ruleToDelete.id
			);
		} else {
			await AdminService.deleteAccessControlRule(ruleToDelete.id);
		}
		accessControlRules = await AdminService.listAccessControlRules();

		ruleToDelete = undefined;
	}}
	oncancel={() => (ruleToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Access Control</title>
</svelte:head>
