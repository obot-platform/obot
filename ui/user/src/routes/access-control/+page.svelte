<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { BookOpenText, ChevronLeft, Plus, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { afterNavigate, goto } from '$app/navigation';
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

	let { data } = $props();
	const { accessControlRules: initialRules, workspaceId } = data;

	initMcpServerAndEntries();

	const mcpServersAndEntries = getPoweruserWorkspace();
	let accessControlRules = $state(initialRules);
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
		goto(`/access-control/${rule.id}`, { replaceState: false });
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

	afterNavigate(() => {
		const url = new URL(window.location.href);
		const queryParams = new URLSearchParams(url.search);
		if (queryParams.get('new')) {
			showCreateRule = true;
		} else {
			showCreateRule = false;
		}
	});

	let title = $derived(showCreateRule ? 'Create Access Control Rule' : 'Access Control');
</script>

<Layout showUserLinks {title} showBackButton={showCreateRule}>
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
						<h4 class="text-on-surface1 text-lg font-semibold">No created access control rules.</h4>
						<p class="text-on-surface1 text-sm font-light">
							Looks like you don't have any access control rules created yet. <br />
							Click the button below to get started.
						</p>

						{@render addRuleButton()}
					</div>
				{:else}
					<Table
						data={accessControlRules}
						fields={['displayName', 'servers']}
						onClickRow={(d, isCtrlClick) => {
							const url = `/access-control/${d.id}`;
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
			goto(`/access-control?new=true`);
		}}
	>
		<Plus class="size-4" /> Add New Rule
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
	msg="Are you sure you want to delete this access control rule?"
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
	<title>Obot | Access Control</title>
</svelte:head>
