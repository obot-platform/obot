<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import { m } from '$lib/i18n';
	import { Role } from '$lib/services/admin/types';
	import type { GroupAssignment } from './types';

	interface Props {
		groupAssignment?: GroupAssignment;
		loading?: boolean;
		onsuccess: (groupAssignment: GroupAssignment) => void;
		oncancel: () => void;
	}

	let { groupAssignment = $bindable(), loading = false, onsuccess, oncancel }: Props = $props();

	const auditorReadonlyAdminRoles = [Role.BASIC, Role.POWERUSER, Role.POWERUSER_PLUS];
	const roleId = $derived(groupAssignment ? groupAssignment.assignment.role & ~Role.AUDITOR : 0);
</script>

<Confirm
	title={m.identity_access_users_confirm_auditor_title()}
	{loading}
	show={Boolean(groupAssignment)}
	onsuccess={async () => {
		if (!groupAssignment) return;
		onsuccess(groupAssignment);
	}}
	{oncancel}
	type="info"
	msg={m.identity_access_groups_grant_auditor_msg({ name: `${groupAssignment?.group.name}` })}
>
	{#snippet note()}
		<div class="my-4 flex flex-col gap-4 text-center">
			<p>
				{#if auditorReadonlyAdminRoles.includes(roleId)}
					{m.identity_access_groups_auditor_note_readonly()}
				{:else}
					{m.identity_access_groups_auditor_note()}
				{/if}
			</p>
			<p>
				{m.identity_access_groups_grant_confirm_prefix()}
				<b>{groupAssignment?.group.name}</b>
				{m.identity_access_groups_grant_confirm_suffix()}
			</p>
		</div>
	{/snippet}
</Confirm>
