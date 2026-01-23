<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { BookOpenText, Plus, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { goto, replaceState } from '$lib/url';
	import { afterNavigate } from '$app/navigation';
	import { type AccessControlRule } from '$lib/services/admin/types';
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
	import { page } from '$app/state';

	let { data } = $props();
	let { accessControlRules, workspaceId } = $derived(data);

	initMcpServerAndEntries();

	const mcpServersAndEntries = getPoweruserWorkspace();
	let showCreateRule = $state(false);
	let ruleToDelete = $state<AccessControlRule>();

	onMount(() => {
		const url = new URL(window.location.href);
		const queryParams = new URLSearchParams(url.search);
		if (queryParams.get('new')) {
			showCreateRule = true;
		}
	});

	async function navigateToCreated(rule: AccessControlRule) {
		showCreateRule = false;
		goto(`/mcp-registries/${rule.id}`, { replaceState: false });
	}

	const duration = PAGE_TRANSITION_DURATION;
	const totalServers = $derived(
		mcpServersAndEntries.entries.length + mcpServersAndEntries.servers.length
	);

	onMount(async () => {
		if (workspaceId) {
			fetchMcpServerAndEntries(workspaceId);
		}
	});

	afterNavigate(({ from }) => {
		const comingFromRegistryPage = from?.url?.pathname.startsWith('/mcp-registries/');
		if (comingFromRegistryPage) {
			showCreateRule = false;
			if (page.url.searchParams.has('new')) {
				const cleanUrl = new URL(page.url);
				cleanUrl.searchParams.delete('new');
				replaceState(cleanUrl, {});
			}
			return;
		} else {
			if (page.url.searchParams.has('new')) {
				showCreateRule = true;
			} else {
				showCreateRule = false;
			}
		}
	});

	let title = $derived(showCreateRule ? 'Create MCP Registry' : 'MCP Registries');
</script>

<Layout {title} showBackButton={showCreateRule}>
	{#snippet rightNavActions()}
		{@render addRuleButton()}
	{/snippet}
	<div
		class="h-full w-full"
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
				{#if accessControlRules.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<BookOpenText class="text-on-surface1 size-24 opacity-25" />
						<h4 class="text-on-surface1 text-lg font-semibold">No created MCP registries.</h4>
						<p class="text-on-surface1 text-sm font-light">
							Looks like you don't have any registries created yet. <br />
							Click the button below to get started.
						</p>

						{@render addRuleButton()}
					</div>
				{:else}
					<Table
						data={accessControlRules}
						fields={['displayName', 'servers']}
						onClickRow={(d, isCtrlClick) => {
							const url = `/mcp-registries/${d.id}`;
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
</Layout>

{#snippet addRuleButton()}
	<button
		class="button-primary flex items-center gap-1 text-sm"
		onclick={() => {
			goto(`/mcp-registries?new=true`);
		}}
	>
		<Plus class="size-4" /> Add New Registry
	</button>
{/snippet}

{#snippet createRuleScreen()}
	<div
		class="h-full w-full"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<AccessControlRuleForm
			onCreate={navigateToCreated}
			entity="workspace"
			id={workspaceId}
			mcpEntriesContextFn={getPoweruserWorkspace}
			all={MCP_PUBLISHER_ALL_OPTION}
		/>
	</div>
{/snippet}

<Confirm
	msg={`Delete ${ruleToDelete?.displayName || 'this MCP registry'}?`}
	show={Boolean(ruleToDelete)}
	onsuccess={async () => {
		if (!ruleToDelete || !workspaceId) return;
		await ChatService.deleteWorkspaceAccessControlRule(workspaceId, ruleToDelete.id);
		accessControlRules = await ChatService.listWorkspaceAccessControlRules(workspaceId);
		ruleToDelete = undefined;
	}}
	oncancel={() => (ruleToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | MCP Registries</title>
</svelte:head>
