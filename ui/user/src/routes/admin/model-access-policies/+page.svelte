<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { LockKeyhole, Plus, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { goto, replaceState } from '$lib/url';
	import { afterNavigate } from '$app/navigation';
	import { type ModelAccessPolicy } from '$lib/services/admin/types';
	import Confirm from '$lib/components/Confirm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import ModelAccessPolicyForm from '$lib/components/admin/ModelAccessPolicyForm.svelte';
	import { onMount, untrack } from 'svelte';
	import { AdminService } from '$lib/services/index.js';
	import { openUrl } from '$lib/utils.js';
	import { profile } from '$lib/stores/index.js';
	import { page } from '$app/state';

	let { data } = $props();
	let modelAccessPolicies = $state(untrack(() => data.modelAccessPolicies));
	let showCreatePolicy = $state(false);
	let policyToDelete = $state<ModelAccessPolicy>();

	function convertToTableData(policy: ModelAccessPolicy) {
		const hasEverything = policy.models?.find((m) => m.id === '*');
		const count = hasEverything ? 'All' : (policy.models?.length ?? 0);

		return {
			...policy,
			modelsCount: count
		};
	}

	let tableData = $derived(modelAccessPolicies.map((d) => convertToTableData(d)));

	let isReadonly = $derived(profile.current.isAdminReadonly?.());

	onMount(() => {
		const url = new URL(window.location.href);
		const queryParams = new URLSearchParams(url.search);
		if (queryParams.get('new')) {
			showCreatePolicy = true;
		}
	});

	afterNavigate(({ from }) => {
		const comingFromPolicyPage = from?.url?.pathname.startsWith('/admin/model-access-policies/');
		if (comingFromPolicyPage) {
			showCreatePolicy = false;
			if (page.url.searchParams.has('new')) {
				const cleanUrl = new URL(page.url);
				cleanUrl.searchParams.delete('new');
				replaceState(cleanUrl, {});
			}
			return;
		} else {
			if (page.url.searchParams.has('new')) {
				showCreatePolicy = true;
			} else {
				showCreatePolicy = false;
			}
		}
	});

	async function navigateToCreated(policy: ModelAccessPolicy) {
		showCreatePolicy = false;
		goto(`/admin/model-access-policies/${policy.id}`, { replaceState: false });
	}

	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(showCreatePolicy ? 'Create Model Access Policy' : 'Model Access Policies');
</script>

<Layout {title} showBackButton={showCreatePolicy}>
	<div
		class="h-full w-full"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if showCreatePolicy}
			{@render createPolicyScreen()}
		{:else}
			<div
				class="flex flex-col gap-8"
				in:fly={{ x: 100, delay: duration, duration }}
				out:fly={{ x: -100, duration }}
			>
				{#if modelAccessPolicies.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<LockKeyhole class="text-on-surface1 size-24 opacity-25" />
						<h4 class="text-on-surface1 text-lg font-semibold">No model access policies</h4>
						<p class="text-on-surface1 text-sm font-light">
							Looks like you don't have any model access policies created yet. <br />
							{#if !isReadonly}
								Click the button below to get started.
							{/if}
						</p>

						{@render addPolicyButton()}
					</div>
				{:else}
					<div class="flex flex-col gap-2">
						<h4 class="text-lg font-semibold">Model Access Policies</h4>
						{@render modelAccessPolicyTable()}
					</div>
				{/if}
			</div>
		{/if}
	</div>

	{#snippet rightNavActions()}
		{#if !showCreatePolicy}
			<div class="relative flex items-center gap-4">
				{@render addPolicyButton()}
			</div>
		{/if}
	{/snippet}
</Layout>

{#snippet modelAccessPolicyTable()}
	<Table
		data={tableData}
		fields={['displayName', 'modelsCount']}
		onClickRow={(d, isCtrlClick) => {
			const url = `/admin/model-access-policies/${d.id}`;
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
						policyToDelete = d;
					}}
					use:tooltip={'Delete Policy'}
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

{#snippet addPolicyButton()}
	{#if !profile.current.isAdminReadonly?.()}
		<button
			class="button-primary flex items-center gap-1 text-sm"
			onclick={() => {
				goto(`/admin/model-access-policies?new=true`);
			}}
		>
			<Plus class="size-4" /> Add New Policy
		</button>
	{/if}
{/snippet}

{#snippet createPolicyScreen()}
	<div
		class="h-full w-full"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<ModelAccessPolicyForm onCreate={navigateToCreated} />
	</div>
{/snippet}

<Confirm
	msg="Are you sure you want to delete this policy?"
	show={Boolean(policyToDelete)}
	onsuccess={async () => {
		if (!policyToDelete) return;
		await AdminService.deleteModelAccessPolicy(policyToDelete.id);
		modelAccessPolicies = await AdminService.listModelAccessPolicies();
		policyToDelete = undefined;
	}}
	oncancel={() => (policyToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Model Access Policies</title>
</svelte:head>
