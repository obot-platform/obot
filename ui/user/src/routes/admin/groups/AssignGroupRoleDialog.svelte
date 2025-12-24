<script lang="ts">
	import { LoaderCircle, Group as GroupIcon } from 'lucide-svelte';

	import { Role } from '$lib/services/admin/types';
	import { getUserRoleLabel } from '$lib/utils';

	import GroupRoleForm from './GroupRoleForm.svelte';
	import type { GroupAssignment } from './types';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';

	interface Props {
		groupAssignment?: GroupAssignment;
		loading?: boolean;
		onClose: () => void;
		onConfirm: (groupAssignment: GroupAssignment) => void;
		onAuditorConfirm: (groupAssignment: GroupAssignment) => void;
		onOwnerConfirm: (groupAssignment: GroupAssignment) => void;
	}

	// Helper functions to work with roles
	function getRoleId(role: number): number {
		return role & ~Role.AUDITOR;
	}

	function hasAuditorFlag(role: number): boolean {
		return (role & Role.AUDITOR) !== 0;
	}

	function addAuditorFlag(role: number): number {
		return role | Role.AUDITOR;
	}

	let {
		groupAssignment = $bindable(),
		loading = false,
		onClose,
		onConfirm,
		onAuditorConfirm,
		onOwnerConfirm
	}: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();

	let draftRoleId = $state(0);
	let draftHaveAuditorPrivilege = $state(false);

	const hasRoleChanged = $derived(
		draftRoleId !== getRoleId(groupAssignment ? groupAssignment.assignment.role : 0)
	);

	const hasAuditorChanged = $derived(
		hasAuditorFlag(groupAssignment ? groupAssignment.assignment.role : 0) !==
			draftHaveAuditorPrivilege
	);

	// Check if any changes were made
	const hasChanges = $derived(hasRoleChanged || hasAuditorChanged);

	$effect(() => {
		if (groupAssignment) {
			// Initialize draft values from assignment
			const role = groupAssignment.assignment.role || 0;
			draftRoleId = getRoleId(role);
			draftHaveAuditorPrivilege = hasAuditorFlag(role);

			dialog?.open();
		}
	});

	function handleClose() {
		onClose();
	}

	function handleConfirm() {
		if (!groupAssignment) return;

		const role = draftHaveAuditorPrivilege ? addAuditorFlag(draftRoleId) : draftRoleId;
		const result: GroupAssignment = {
			group: groupAssignment.group,
			assignment: {
				groupName: groupAssignment.group.name,
				role
			}
		};

		// Only description changed - update directly
		if (!hasRoleChanged && !hasAuditorChanged) {
			onConfirm(result);
			return;
		}

		// Auditor changed - show auditor confirmation
		if (hasAuditorChanged && draftHaveAuditorPrivilege && draftRoleId !== 0) {
			onAuditorConfirm(result);
			return;
		}

		// Changing to owner role - show owner confirmation
		const currentRoleId = getRoleId(groupAssignment.assignment.role || 0);
		if (draftRoleId === Role.OWNER && currentRoleId !== Role.OWNER) {
			onOwnerConfirm(result);
			return;
		}

		onConfirm(result);
	}
</script>

{#if groupAssignment}
	<ResponsiveDialog
		bind:this={dialog}
		onClose={handleClose}
		class="flex max-h-[90svh] w-full max-w-[94svw] flex-col overflow-visible md:max-w-xl"
		classes={{ content: 'p-4', header: 'mb-4' }}
	>
		{#snippet titleContent()}
			<div class="flex w-full flex-col gap-3">
				<span class="block text-center text-lg font-semibold md:text-start md:text-xl">
					{groupAssignment.assignment.role ? 'Update' : 'Assign'} Group Role
				</span>
			</div>
		{/snippet}

		{#if groupAssignment.assignment.role}
			<div class="dark:bg-surface1 mb-8 flex flex-col gap-1 rounded-lg bg-gray-50 p-3">
				<div class="flex items-center gap-2">
					{#if groupAssignment.group.iconURL}
						<img
							src={groupAssignment.group.iconURL}
							alt={groupAssignment.group.name}
							class="size-5 rounded-full"
						/>
					{:else}
						<GroupIcon class="text-on-surface1 size-5" />
					{/if}
					<span class="font-semibold">{groupAssignment.group.name}</span>
				</div>
				<div class="text-on-surface1 text-xs">
					Current: {getUserRoleLabel(groupAssignment.assignment.role)}
				</div>
			</div>
		{/if}

		<div class="flex-1 overflow-y-auto pr-2">
			<GroupRoleForm
				bind:roleId={draftRoleId}
				bind:hasAuditorPrivilege={draftHaveAuditorPrivilege}
			/>
		</div>

		<div class="mt-4 flex flex-shrink-0 justify-end gap-2">
			<button class="button" onclick={handleClose}>Cancel</button>
			<button
				class="button-primary"
				onclick={handleConfirm}
				disabled={loading || (!!groupAssignment.assignment.role && !hasChanges)}
			>
				{#if loading}
					<LoaderCircle class="size-4 animate-spin" />
				{:else}
					{groupAssignment.assignment.role ? 'Update' : 'Assign'}
				{/if}
			</button>
		</div>
	</ResponsiveDialog>
{/if}
