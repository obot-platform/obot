<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
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
	{loading}
	show={Boolean(groupAssignment)}
	onsuccess={async () => {
		if (!groupAssignment) return;
		onsuccess(groupAssignment);
	}}
	{oncancel}
>
	{#snippet title()}
		<div class="flex items-center gap-2">
			<h3 class="text-xl font-semibold">Confirm Auditor Role for Group</h3>
		</div>
	{/snippet}
	{#snippet note()}
		<div class="mt-4 mb-8 flex flex-col gap-4 text-center">
			<p>
				{#if auditorReadonlyAdminRoles.includes(roleId)}
					All members of this group will have read-only access to the admin system and can see
					additional details such as response, request, and header information in the audit logs.
				{:else}
					All members of this group will gain access to additional details such as response,
					request, and header information in the audit logs.
				{/if}
			</p>
			<p>
				Are you sure you want to grant the <b>{groupAssignment?.group.name}</b> group this role?
			</p>
		</div>
	{/snippet}
</Confirm>
