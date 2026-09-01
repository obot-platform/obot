<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import ModelAccessPolicyForm from '$lib/components/admin/ModelAccessPolicyForm.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { type ModelAccessPolicy } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services/index.js';
	import { accessibleModels, profile } from '$lib/stores/index.js';
	import { goto, clearUrlParams } from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import { LockKeyhole, Plus, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fade, fly } from 'svelte/transition';

	interface Props {
		modelAccessPolicies: ModelAccessPolicy[];
		creating?: boolean;
	}

	let { modelAccessPolicies: initialPolicies, creating = false }: Props = $props();
	let modelAccessPolicies = $state(untrack(() => initialPolicies));
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
		await accessibleModels.refresh();
		clearUrlParams(['new']);
		goto(`/models/access-policies/${policy.id}`, { replaceState: false });
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

{#if creating}
	{@render createPolicyScreen()}
{:else}
	<div class="flex flex-col gap-8" in:fade={{ duration }}>
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

{#snippet modelAccessPolicyTable()}
	<Table
		data={tableData}
		fields={['displayName', 'modelsCount']}
		onClickRow={(d, isCtrlClick) => {
			const url = `/models/access-policies/${d.id}`;
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
				goto(`${page.url.pathname}?view=access-policies&new=true`);
			}}
		>
			<Plus class="size-4" /> Add Access Policy
		</button>
	{/if}
{/snippet}

{#snippet createPolicyScreen()}
	<div class="h-full w-full" in:fly={{ x: 100, delay: duration, duration }}>
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
		await accessibleModels.refresh();
		policyToDelete = undefined;
	}}
	oncancel={() => (policyToDelete = undefined)}
/>
