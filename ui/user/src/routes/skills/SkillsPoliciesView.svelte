<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { m } from '$lib/i18n';
	import { type SkillAccessPolicy } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services/index.js';
	import { profile } from '$lib/stores/index.js';
	import { goto } from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import { Plus, Trash2, Vault } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		skillAccessPolicies: SkillAccessPolicy[];
	}

	let { skillAccessPolicies: initialPolicies }: Props = $props();
	let skillAccessPolicies = $state(untrack(() => initialPolicies));
	let policyToDelete = $state<SkillAccessPolicy>();

	let isReadonly = $derived(profile.current.isAdminReadonly?.());

	const duration = PAGE_TRANSITION_DURATION;
</script>

<div class="flex flex-col gap-8" in:fade={{ duration }}>
	{#if skillAccessPolicies.length === 0}
		<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<Vault class="text-muted-content size-24 opacity-25" />
			<h4 class="text-muted-content text-lg font-semibold">
				{m.skills_no_access_policies()}
			</h4>
			<p class="text-muted-content text-sm font-light">
				{m.skills_no_access_policies_desc()} <br />
				{#if !isReadonly}
					{m.core_click_below_to_start()}
				{/if}
			</p>

			{@render addPolicyButton()}
		</div>
	{:else}
		<div class="flex flex-col gap-2">
			{@render skillAccessPolicyTable()}
		</div>
	{/if}
</div>

{#snippet skillAccessPolicyTable()}
	<Table
		data={skillAccessPolicies}
		fields={['displayName']}
		headers={[{ property: 'displayName', title: m.core_name() }]}
		onClickRow={(d, isCtrlClick) => {
			const url = `/skills/access-policies/${d.id}`;
			openUrl(url, isCtrlClick);
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
					tooltip={{ text: m.core_delete_policy() }}
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
				goto(
					`${page.url.pathname}?view=${page.url.searchParams.get('view') ?? 'access-policies'}&new=true`
				);
			}}
		>
			<Plus class="size-4" />
			{m.skills_add_access_policy()}
		</button>
	{/if}
{/snippet}

<Confirm
	msg={m.skills_delete_named({
		name: policyToDelete?.displayName || m.skills_this_policy()
	})}
	show={Boolean(policyToDelete)}
	onsuccess={async () => {
		if (!policyToDelete) return;
		await AdminService.deleteSkillAccessPolicy(policyToDelete.id);
		skillAccessPolicies = await AdminService.listSkillAccessPolicies();
		policyToDelete = undefined;
	}}
	oncancel={() => (policyToDelete = undefined)}
/>
