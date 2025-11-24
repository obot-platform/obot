<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import type { GroupAssignment } from './types';

	interface Props {
		groupAssignment?: GroupAssignment;
		loading?: boolean;
		onsuccess: (groupAssignment: GroupAssignment) => void;
		oncancel: () => void;
	}

	let { groupAssignment = $bindable(), loading = false, onsuccess, oncancel }: Props = $props();
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
			<h3 class="text-xl font-semibold">Confirm Owner Role Assignment</h3>
		</div>
	{/snippet}

	{#snippet note()}
		<div class="mt-4 mb-8 flex flex-col gap-4">
			<p class="text-left font-semibold text-orange-600 dark:text-orange-400">
				Warning: Assigning the Owner role to a group grants extensive privileges.
			</p>
			<div class="text-left text-sm">
				<p class="mb-2">All members of <b>{groupAssignment?.group.name}</b> will be able to:</p>
				<ul class="ml-6 list-disc space-y-1">
					<li>Manage all aspects of the platform</li>
					<li>Assign roles to other users and groups, including the Owner role</li>
					<li>Assign the Auditor role (a privilege unique to Owners)</li>
					<li>Access and modify all system configurations</li>
					<li>Delete users and manage authentication providers</li>
				</ul>
			</div>
			<p class="text-left text-sm">
				This feature is currently under review. Please ensure you understand the implications before
				proceeding.
			</p>
			<p class="text-left font-semibold">
				Are you sure you want to assign the Owner role to this group?
			</p>
		</div>
	{/snippet}
</Confirm>
