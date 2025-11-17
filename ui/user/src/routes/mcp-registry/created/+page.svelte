<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { BookOpenText, ChevronLeft, Plus, ServerCog, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { afterNavigate, goto } from '$app/navigation';
	import { Group, type AccessControlRule } from '$lib/services/admin/types';
	import Confirm from '$lib/components/Confirm.svelte';
	import { MCP_PUBLISHER_ALL_OPTION, PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import AccessControlRuleForm from '$lib/components/admin/AccessControlRuleForm.svelte';
	import { onMount } from 'svelte';
	import { ChatService } from '$lib/services/index.js';
	import { openUrl } from '$lib/utils.js';
	import {
		fetchMcpServerAndEntries,
		getPoweruserWorkspace,
		initMcpServerAndEntries
	} from '$lib/context/poweruserWorkspace.svelte.js';
	import { workspaceStore } from '$lib/stores/workspace.svelte.js';
	import { profile } from '$lib/stores';
	import { browser } from '$app/environment';

	initMcpServerAndEntries();

	const mcpServersAndEntries = getPoweruserWorkspace();
	let showCreateRule = $state(false);
	let ruleToDelete = $state<AccessControlRule>();
	let workspace = $derived($workspaceStore);
	let hasAccessToCreateRegistry = $derived(profile.current?.groups?.includes(Group.POWERUSER_PLUS));

	onMount(() => {
		const url = new URL(window.location.href);
		const queryParams = new URLSearchParams(url.search);
		if (queryParams.get('new')) {
			showCreateRule = true;
		}
	});

	afterNavigate(({ to }) => {
		if (browser && to?.url) {
			const showCreate = to.url.searchParams.get('new') === 'true';
			if (showCreate) {
				showCreateRule = true;
			} else {
				showCreateRule = false;
			}
		}
	});

	async function navigateToCreated(rule: AccessControlRule) {
		showCreateRule = false;
		goto(`/mcp-registry/r/${rule.id}`, { replaceState: false });
	}

	const duration = PAGE_TRANSITION_DURATION;
	const totalServers = $derived(
		mcpServersAndEntries.entries.length + mcpServersAndEntries.servers.length
	);

	$effect(() => {
		if (workspace.id) {
			fetchMcpServerAndEntries(workspace.id);
		}
	});

	let title = $derived(showCreateRule ? 'Create New Registry' : 'Created by Me');
</script>

<Layout {title} showBackButton={showCreateRule}>
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
				{#if workspace.rules.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<ServerCog class="size-24 text-gray-200 dark:text-gray-900" />
						<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
							No created registries
						</h4>
						<p class="text-sm font-light text-gray-400 dark:text-gray-600">
							Looks like you don't have any registries created yet. <br />
							Click the button below to get started.
						</p>

						<button
							class="button-primary flex items-center gap-1 text-sm"
							onclick={() => (showCreateRule = true)}
						>
							<Plus class="size-4" /> New Registry
						</button>
					</div>
				{:else}
					<Table
						data={workspace.rules}
						fields={['displayName', 'servers']}
						onClickRow={(d, isCtrlClick) => {
							const url = `/mcp-registry/r/${d.id}`;
							openUrl(url, isCtrlClick);
						}}
						headers={[
							{
								title: 'Name',
								property: 'displayName'
							}
						]}
					>
						{#snippet actions(d)}
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
						{/snippet}
						{#snippet onRenderColumn(property, d)}
							{#if property === 'servers'}
								{@const hasEverything = d.resources?.find((r) => r.id === '*')}
								{@const count = hasEverything
									? totalServers
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
	</div>

	{#snippet rightNavActions()}
		{#if hasAccessToCreateRegistry && !showCreateRule}
			<button
				class="button-primary flex h-fit items-center gap-2 text-sm"
				onclick={() => goto('/mcp-registry/created?new=true')}
			>
				<Plus class="size-4" />
				New Registry
			</button>
		{/if}
	{/snippet}
</Layout>

{#snippet createRuleScreen()}
	<div
		class="h-full w-full"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<AccessControlRuleForm
			onCreate={navigateToCreated}
			entity="workspace"
			id={workspace.id}
			mcpEntriesContextFn={getPoweruserWorkspace}
			all={MCP_PUBLISHER_ALL_OPTION}
		/>
	</div>
{/snippet}

<Confirm
	msg="Are you sure you want to delete this registry?"
	show={Boolean(ruleToDelete)}
	onsuccess={async () => {
		if (!ruleToDelete || !workspace.id) return;
		await ChatService.deleteWorkspaceAccessControlRule(workspace.id, ruleToDelete.id);
		await workspaceStore.fetchData(true);
		ruleToDelete = undefined;
	}}
	oncancel={() => (ruleToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Created by Me</title>
</svelte:head>
