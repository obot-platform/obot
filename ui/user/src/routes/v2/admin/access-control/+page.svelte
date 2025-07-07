<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/Table.svelte';
	import { BookOpenText, ChevronLeft, Plus, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { type AccessControlRule } from '$lib/services/admin/types';
	import Confirm from '$lib/components/Confirm.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import AccessControlRuleForm from '$lib/components/admin/AccessControlRuleForm.svelte';
	import { onMount } from 'svelte';
	import {
		fetchMcpServerAndEntries,
		getAdminMcpServerAndEntries,
		initMcpServerAndEntries
	} from '$lib/context/admin/mcpServerAndEntries.svelte';
	import { AdminService } from '$lib/services/index.js';

	let { data } = $props();
	const { accessControlRules: initialRules } = data;
	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;

	initMcpServerAndEntries();

	const mcpServersAndEntries = getAdminMcpServerAndEntries();
	let accessControlRules = $state(initialRules);
	let showCreateRule = $state(false);
	let ruleToDelete = $state<AccessControlRule>();

	function handleNavigation(url: string) {
		goto(url, { replaceState: false });
	}

	async function navigateToCreated(rule: AccessControlRule) {
		showCreateRule = false;
		goto(`/v2/admin/access-control/${rule.id}`, { replaceState: false });
	}

	const duration = PAGE_TRANSITION_DURATION;
	const totalServers = $derived(
		mcpServersAndEntries.entries.length + mcpServersAndEntries.servers.length
	);

	onMount(async () => {
		fetchMcpServerAndEntries(defaultCatalogId);
	});
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
							Click the button below to get started.
						</p>

						{@render addRuleButton()}
					</div>
				{:else}
					<Table
						data={accessControlRules}
						fields={['displayName', 'servers']}
						onSelectRow={(d) => {
							handleNavigation(`/v2/admin/access-control/${d.id}`);
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
		onclick={() => (showCreateRule = true)}
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
		<AccessControlRuleForm onCreate={navigateToCreated}>
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
	msg={'Are you sure you want to delete this rule?'}
	show={Boolean(ruleToDelete)}
	onsuccess={async () => {
		if (!ruleToDelete) return;
		await AdminService.deleteAccessControlRule(ruleToDelete.id);
		accessControlRules = await AdminService.listAccessControlRules();
		ruleToDelete = undefined;
	}}
	oncancel={() => (ruleToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Access Control</title>
</svelte:head>
