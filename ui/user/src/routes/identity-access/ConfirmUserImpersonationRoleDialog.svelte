<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import { m } from '$lib/i18n';
	import { Role } from '$lib/services/admin/types';
	import type { GroupAssignment } from './types';

	interface Props {
		groupAssignment?: GroupAssignment;
		currentRole?: number;
		loading?: boolean;
		onsuccess: (groupAssignment: GroupAssignment) => void;
		oncancel: () => void;
	}

	let {
		groupAssignment = $bindable(),
		currentRole = 0,
		loading = false,
		onsuccess,
		oncancel
	}: Props = $props();

	const addingAuditor = $derived(
		Boolean(
			groupAssignment &&
			(groupAssignment.assignment.role & Role.AUDITOR) !== 0 &&
			(currentRole & Role.AUDITOR) === 0
		)
	);
	const removingAuditor = $derived(
		Boolean(
			groupAssignment &&
			(groupAssignment.assignment.role & Role.AUDITOR) === 0 &&
			(currentRole & Role.AUDITOR) !== 0
		)
	);
</script>

<Confirm
	title={m.identity_access_users_confirm_impersonator_title()}
	{loading}
	show={Boolean(groupAssignment)}
	onsuccess={async () => {
		if (!groupAssignment) return;
		onsuccess(groupAssignment);
	}}
	{oncancel}
	type="info"
	msg={m.identity_access_groups_grant_impersonator_msg({ name: `${groupAssignment?.group.name}` })}
>
	{#snippet note()}
		<div class="my-4 flex flex-col gap-4 text-center">
			<p>
				{m.identity_access_groups_impersonator_note()}
			</p>
			{#if addingAuditor}
				<p>
					{m.identity_access_users_impersonator_note_adds_auditor()}
				</p>
			{:else if removingAuditor}
				<p>{m.identity_access_users_impersonator_note_removes_auditor()}</p>
			{/if}
			<p>
				{m.identity_access_groups_grant_confirm_prefix()}
				<b>{groupAssignment?.group.name}</b>
				{m.identity_access_groups_grant_confirm_suffix()}
			</p>
		</div>
	{/snippet}
</Confirm>
