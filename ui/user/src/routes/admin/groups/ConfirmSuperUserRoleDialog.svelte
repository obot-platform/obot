<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
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
		Boolean(groupAssignment && (groupAssignment.assignment.role & Role.AUDITOR) !== 0 && (currentRole & Role.AUDITOR) === 0)
	);
	const removingAuditor = $derived(
		Boolean(groupAssignment && (groupAssignment.assignment.role & Role.AUDITOR) === 0 && (currentRole & Role.AUDITOR) !== 0)
	);
</script>

<Confirm
	title="Confirm Super User Role"
	{loading}
	show={Boolean(groupAssignment)}
	onsuccess={async () => {
		if (!groupAssignment) return;
		onsuccess(groupAssignment);
	}}
	{oncancel}
	type="info"
	msg={`Grant ${groupAssignment?.group.name} the Super User role?`}
>
	{#snippet note()}
		<div class="mt-4 mb-8 flex flex-col gap-4 text-center">
			<p>
				Super User grants elevated cross-user MCP access so members of this group can connect to
				other users' MCP servers.
			</p>
			{#if addingAuditor}
				<p>
					This update will also grant Auditor, which adds expanded audit visibility (including
					request/response/header details).
				</p>
			{:else if removingAuditor}
				<p>
					This update will remove Auditor while granting Super User access.
				</p>
			{/if}
			<p>
				Are you sure you want to grant the <b>{groupAssignment?.group.name}</b> group this role?
			</p>
		</div>
	{/snippet}
</Confirm>
