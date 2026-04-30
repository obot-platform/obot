<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ModelAccessPolicyForm from '$lib/components/admin/ModelAccessPolicyForm.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { type ModelAccessPolicy } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services/index.js';
	import { profile } from '$lib/stores/index.js';
	import { goto, clearUrlParams } from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import { LockKeyhole, Plus, Trash2 } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let modelAccessPolicies = $state(untrack(() => data.modelAccessPolicies));
	let showCreatePolicy = $derived(page.url.searchParams.has('new'));
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

	async function navigateToCreated(policy: ModelAccessPolicy) {
		clearUrlParams(['new']);
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
						<LockKeyhole class="text-base-content/80 size-24 opacity-25" />
						<h4 class="text-muted-content text-lg font-semibold">No model access policies</h4>
						<p class="text-muted-content text-sm font-light">
							Looks like you don't have any model access policies created yet. <br />
							{#if !isReadonly}
								Click the button below to get started.
							{/if}
						</p>

						{@render addPolicyButton()}
					</div>
				{:else}
					<div class="flex flex-col gap-2">
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
				<IconButton
					variant="danger"
					onclick={(e) => {
						e.stopPropagation();
						policyToDelete = d;
					}}
					tooltip={{ text: 'Delete Policy' }}
				>
					<Trash2 class="size-4" />
				</IconButton>
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
			class="btn btn-primary flex items-center gap-1 text-sm"
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
	msg={`Delete ${policyToDelete?.displayName || 'this policy'}?`}
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
