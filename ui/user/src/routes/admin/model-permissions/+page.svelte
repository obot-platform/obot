<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { LockKeyhole, Plus, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { goto, replaceState } from '$lib/url';
	import { afterNavigate } from '$app/navigation';
	import { type ModelPermissionRule } from '$lib/services/admin/types';
	import Confirm from '$lib/components/Confirm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import ModelPermissionRuleForm from '$lib/components/admin/ModelPermissionRuleForm.svelte';
	import { onMount, untrack } from 'svelte';
	import { AdminService } from '$lib/services/index.js';
	import { openUrl } from '$lib/utils.js';
	import { profile } from '$lib/stores/index.js';
	import { page } from '$app/state';

	let { data } = $props();
	let modelPermissionRules = $state(untrack(() => data.modelPermissionRules));
	let showCreateRule = $state(false);
	let ruleToDelete = $state<ModelPermissionRule>();

	function convertToTableData(rule: ModelPermissionRule) {
		const hasEverything = rule.models?.find((m) => m.modelID === '*');
		const count = hasEverything ? 'All' : (rule.models?.length ?? 0);

		return {
			...rule,
			modelsCount: count
		};
	}

	let tableData = $derived(modelPermissionRules.map((d) => convertToTableData(d)));

	let isReadonly = $derived(profile.current.isAdminReadonly?.());

	onMount(() => {
		const url = new URL(window.location.href);
		const queryParams = new URLSearchParams(url.search);
		if (queryParams.get('new')) {
			showCreateRule = true;
		}
	});

	afterNavigate(({ from }) => {
		const comingFromRulePage = from?.url?.pathname.startsWith('/admin/model-permissions/');
		if (comingFromRulePage) {
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

	async function navigateToCreated(rule: ModelPermissionRule) {
		showCreateRule = false;
		goto(`/admin/model-permissions/${rule.id}`, { replaceState: false });
	}

	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(showCreateRule ? 'Create Model Permission Rule' : 'Model Permissions');
</script>

<Layout {title} showBackButton={showCreateRule}>
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
				{#if modelPermissionRules.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<LockKeyhole class="text-on-surface1 size-24 opacity-25" />
						<h4 class="text-on-surface1 text-lg font-semibold">No model permission rules</h4>
						<p class="text-on-surface1 text-sm font-light">
							Looks like you don't have any model permission rules created yet. <br />
							{#if !isReadonly}
								Click the button below to get started.
							{/if}
						</p>

						{@render addRuleButton()}
					</div>
				{:else}
					<div class="flex flex-col gap-2">
						<h4 class="text-lg font-semibold">Model Permission Rules</h4>
						{@render modelPermissionRuleTable()}
					</div>
				{/if}
			</div>
		{/if}
	</div>

	{#snippet rightNavActions()}
		{#if !showCreateRule}
			<div class="relative flex items-center gap-4">
				{@render addRuleButton()}
			</div>
		{/if}
	{/snippet}
</Layout>

{#snippet modelPermissionRuleTable()}
	<Table
		data={tableData}
		fields={['displayName', 'modelsCount']}
		onClickRow={(d, isCtrlClick) => {
			const url = `/admin/model-permissions/${d.id}`;
			openUrl(url, isCtrlClick);
		}}
		headers={[
			{
				title: 'Name',
				property: 'displayName'
			},
			{
				title: 'Models',
				property: 'modelsCount'
			}
		]}
		filterable={['displayName']}
		sortable={['displayName', 'modelsCount']}
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
			{#if property === 'modelsCount'}
				{d.modelsCount === 0 ? '-' : d.modelsCount}
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
			onclick={() => {
				goto(`/admin/model-permissions?new=true`);
			}}
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
		<ModelPermissionRuleForm onCreate={navigateToCreated} />
	</div>
{/snippet}

<Confirm
	msg="Are you sure you want to delete this rule?"
	show={Boolean(ruleToDelete)}
	onsuccess={async () => {
		if (!ruleToDelete) return;
		await AdminService.deleteModelPermissionRule(ruleToDelete.id);
		modelPermissionRules = await AdminService.listModelPermissionRules();
		ruleToDelete = undefined;
	}}
	oncancel={() => (ruleToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Model Permissions</title>
</svelte:head>
