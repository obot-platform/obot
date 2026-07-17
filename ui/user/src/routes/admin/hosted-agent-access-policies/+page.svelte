<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import HostedAgentAccessPolicyForm from '$lib/components/admin/HostedAgentAccessPolicyForm.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { type HostedAgentAccessPolicy } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services/index.js';
	import { profile } from '$lib/stores/index.js';
	import { clearUrlParams, goto } from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import { Plus, Trash2, Vault } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let hostedAgentAccessPolicies = $state(untrack(() => data.hostedAgentAccessPolicies));
	let policyToDelete = $state<HostedAgentAccessPolicy>();

	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	let showCreateNew = $derived(page.url.searchParams.has('new'));

	async function navigateToCreated(policy: HostedAgentAccessPolicy) {
		clearUrlParams(['new']);
		goto(`/admin/hosted-agent-access-policies/${policy.id}`, { replaceState: false });
	}

	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(showCreateNew ? 'Create Agent Access Policy' : 'Agent Access Policies');
</script>

<Layout {title} showBackButton={showCreateNew}>
	<div
		class="h-full w-full"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if showCreateNew}
			{@render createPolicyScreen()}
		{:else}
			<div
				class="flex flex-col gap-8"
				in:fly={{ x: 100, delay: duration, duration }}
				out:fly={{ x: -100, duration }}
			>
				{#if hostedAgentAccessPolicies.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<Vault class="text-muted-content size-24 opacity-25" />
						<h4 class="text-muted-content text-lg font-semibold">No agent access policies</h4>
						<p class="text-muted-content text-sm font-light">
							Looks like you don't have any agent access policies created yet. <br />
							{#if !isReadonly}
								Click the button below to get started.
							{/if}
						</p>

						{@render addPolicyButton()}
					</div>
				{:else}
					<div class="flex flex-col gap-2">
						{@render policyTable()}
					</div>
				{/if}
			</div>
		{/if}
	</div>

	{#snippet rightNavActions()}
		{#if !showCreateNew}
			<div class="relative flex items-center gap-4">
				{@render addPolicyButton()}
			</div>
		{/if}
	{/snippet}
</Layout>

{#snippet policyTable()}
	<Table
		data={hostedAgentAccessPolicies}
		fields={['displayName']}
		headers={[{ property: 'displayName', title: 'Name' }]}
		onClickRow={(d, isCtrlClick) => {
			openUrl(`/admin/hosted-agent-access-policies/${d.id}`, isCtrlClick);
		}}
		sortable={['displayName']}
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
			{d[property as keyof typeof d]}
		{/snippet}
	</Table>
{/snippet}

{#snippet addPolicyButton()}
	{#if !profile.current.isAdminReadonly?.()}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => {
				goto(`/admin/hosted-agent-access-policies?new=true`);
			}}
		>
			<Plus class="size-4" /> Add Access Policy
		</button>
	{/if}
{/snippet}

{#snippet createPolicyScreen()}
	<div
		class="h-full w-full"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<HostedAgentAccessPolicyForm onCreate={navigateToCreated} readonly={isReadonly} />
	</div>
{/snippet}

<Confirm
	msg={`Delete ${policyToDelete?.displayName || 'this policy'}?`}
	show={Boolean(policyToDelete)}
	onsuccess={async () => {
		if (!policyToDelete) return;
		await AdminService.deleteHostedAgentAccessPolicy(policyToDelete.id);
		hostedAgentAccessPolicies = await AdminService.listHostedAgentAccessPolicies();
		policyToDelete = undefined;
	}}
	oncancel={() => (policyToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Agent Access Policies</title>
</svelte:head>
