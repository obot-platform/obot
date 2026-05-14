<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import MessagePolicyForm from '$lib/components/admin/MessagePolicyForm.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { type MessagePolicy, PolicyDirectionLabels } from '$lib/services/admin/types';
	import type { PolicyDirection } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services/index.js';
	import { profile } from '$lib/stores/index.js';
	import { goto, clearUrlParams } from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import { ShieldAlert, Plus, Trash2 } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let messagePolicies = $state(untrack(() => data.messagePolicies));
	let showCreatePolicy = $derived(page.url.searchParams.has('new'));
	let policyToDelete = $state<MessagePolicy>();

	function convertToTableData(policy: MessagePolicy) {
		return {
			...policy,
			directionLabel: PolicyDirectionLabels[policy.direction as PolicyDirection] ?? policy.direction
		};
	}

	let tableData = $derived(messagePolicies.map((d) => convertToTableData(d)));

	let isReadonly = $derived(profile.current.isAdminReadonly?.());

	async function navigateToCreated(policy: MessagePolicy) {
		clearUrlParams(['new']);
		goto(`/admin/message-policies/${policy.id}`, { replaceState: false });
	}

	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(showCreatePolicy ? 'Create Message Policy' : 'Message Policies');
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
				{#if messagePolicies.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<ShieldAlert class="text-on-surface1 size-24 opacity-25" />
						<h4 class="text-on-surface1 text-lg font-semibold">No message policies</h4>
						<p class="text-on-surface1 text-sm font-light">
							Looks like you don't have any message policies created yet. <br />
							{#if !isReadonly}
								Click the button below to get started.
							{/if}
						</p>

						{@render addPolicyButton()}
					</div>
				{:else}
					<div class="flex flex-col gap-2">
						{@render messagePolicyTable()}
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

{#snippet messagePolicyTable()}
	<Table
		data={tableData}
		fields={['displayName', 'directionLabel']}
		onClickRow={(d, isCtrlClick) => {
			const url = `/admin/message-policies/${d.id}`;
			openUrl(url, isCtrlClick);
		}}
		headers={[
			{
				title: 'Name',
				property: 'displayName'
			},
			{
				title: 'Applies to',
				property: 'directionLabel'
			}
		]}
		filterable={['displayName']}
		sortable={['displayName', 'directionLabel']}
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
	</Table>
{/snippet}

{#snippet addPolicyButton()}
	{#if !profile.current.isAdminReadonly?.()}
		<button
			class="button-primary flex items-center gap-1 text-sm"
			onclick={() => {
				goto(`/admin/message-policies?new=true`);
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
		<MessagePolicyForm onCreate={navigateToCreated} />
	</div>
{/snippet}

<Confirm
	msg={`Delete ${policyToDelete?.displayName || 'this policy'}?`}
	show={Boolean(policyToDelete)}
	onsuccess={async () => {
		if (!policyToDelete) return;
		await AdminService.deleteMessagePolicy(policyToDelete.id);
		messagePolicies = await AdminService.listMessagePolicies();
		policyToDelete = undefined;
	}}
	oncancel={() => (policyToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Message Policies</title>
</svelte:head>
