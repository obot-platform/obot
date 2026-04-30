<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { Role } from '$lib/services/admin/types';
	import { getUserRoleLabel } from '$lib/utils';
	import GroupRoleForm from './GroupRoleForm.svelte';
	import type { GroupAssignment } from './types';
	import { Group as GroupIcon } from 'lucide-svelte';

	interface Props {
		groupAssignment?: GroupAssignment;
		loading?: boolean;
		onClose: () => void;
		onConfirm: (groupAssignment: GroupAssignment) => void;
		onAuditorConfirm: (groupAssignment: GroupAssignment) => void;
		onUserImpersonationConfirm: (groupAssignment: GroupAssignment) => void;
		onOwnerConfirm: (groupAssignment: GroupAssignment) => void;
		open?: boolean;
	}

	// Helper functions to work with roles
	function getRoleId(role: number): number {
		return role & ~(Role.AUDITOR | Role.USER_IMPERSONATION);
	}

	function hasAuditorFlag(role: number): boolean {
		return (role & Role.AUDITOR) !== 0;
	}

	function addAuditorFlag(role: number): number {
		return role | Role.AUDITOR;
	}

	function hasUserImpersonationFlag(role: number): boolean {
		return (role & Role.USER_IMPERSONATION) !== 0;
	}

	function addUserImpersonationFlag(role: number): number {
		return role | Role.USER_IMPERSONATION;
	}

	let {
		groupAssignment = $bindable(),
		open,
		loading = false,
		onClose,
		onConfirm,
		onAuditorConfirm,
		onUserImpersonationConfirm,
		onOwnerConfirm
	}: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();

	let draftRoleId = $state(0);
	let draftHaveAuditorPrivilege = $state(false);
	let draftHaveUserImpersonationPrivilege = $state(false);

	const hasRoleChanged = $derived(
		draftRoleId !== getRoleId(groupAssignment ? groupAssignment.assignment.role : 0)
	);

	const hasAuditorChanged = $derived(
		hasAuditorFlag(groupAssignment ? groupAssignment.assignment.role : 0) !==
			draftHaveAuditorPrivilege
	);

	const hasUserImpersonationChanged = $derived(
		hasUserImpersonationFlag(groupAssignment ? groupAssignment.assignment.role : 0) !==
			draftHaveUserImpersonationPrivilege
	);

	// Check if any changes were made
	const hasChanges = $derived(hasRoleChanged || hasAuditorChanged || hasUserImpersonationChanged);

	$effect(() => {
		if (groupAssignment) {
			// Initialize draft values from assignment
			const role = groupAssignment.assignment.role || 0;
			draftRoleId = getRoleId(role);
			draftHaveAuditorPrivilege = hasAuditorFlag(role);
			draftHaveUserImpersonationPrivilege = hasUserImpersonationFlag(role);
		}
	});

	$effect(() => {
		if (open) {
			dialog?.open();
		} else {
			dialog?.close();
		}
	});

	function handleClose() {
		onClose();
	}

	function handleConfirm() {
		if (!groupAssignment) return;

		let role = draftRoleId;
		if (draftHaveAuditorPrivilege) {
			role = addAuditorFlag(role);
		}
		if (draftHaveUserImpersonationPrivilege) {
			role = addUserImpersonationFlag(role);
		}
		const result: GroupAssignment = {
			group: groupAssignment.group,
			assignment: {
				groupName: groupAssignment.group.id,
				role
			}
		};

		const currentRoleId = getRoleId(groupAssignment.assignment.role || 0);
		if (hasUserImpersonationChanged && draftHaveUserImpersonationPrivilege && draftRoleId !== 0) {
			// User Impersonation changed - show confirmation
			onUserImpersonationConfirm(result);
		} else if (hasAuditorChanged && draftHaveAuditorPrivilege && draftRoleId !== 0) {
			// Auditor changed - show auditor confirmation
			onAuditorConfirm(result);
		} else if (draftRoleId === Role.OWNER && currentRoleId !== Role.OWNER) {
			// Changing to owner role - show owner confirmation
			onOwnerConfirm(result);
		} else {
			onConfirm(result);
		}
		onClose();
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	onClose={handleClose}
	class="flex max-h-[90svh] w-full max-w-[94svw] flex-col overflow-visible md:max-w-xl"
	classes={{ content: 'p-4 overflow-y-auto overflow-x-hidden flex-1', header: 'mb-4' }}
>
	{#snippet titleContent()}
		<div class="flex w-full flex-col gap-3">
			<span class="block text-center text-lg font-semibold md:text-start md:text-xl">
				{groupAssignment?.assignment.role ? 'Update' : 'Assign'} Group Role
			</span>
		</div>
	{/snippet}

	{#if groupAssignment}
		{#if groupAssignment.assignment.role}
			<div class="dark:bg-base-200 mb-8 flex flex-col gap-1 rounded-lg bg-gray-50 p-3">
				<div class="flex items-center gap-2">
					{#if groupAssignment.group.iconURL}
						<img
							src={groupAssignment.group.iconURL}
							alt={groupAssignment.group.name}
							class="size-5 rounded-full"
						/>
					{:else}
						<GroupIcon class="text-muted-content size-5" />
					{/if}
					<span class="font-semibold">{groupAssignment.group.name}</span>
				</div>
				<div class="text-muted-content text-xs">
					Current: {getUserRoleLabel(groupAssignment.assignment.role)}
				</div>
			</div>
		{/if}

		<div class="flex-1 overflow-y-auto pr-2">
			<GroupRoleForm
				bind:roleId={draftRoleId}
				bind:hasAuditorPrivilege={draftHaveAuditorPrivilege}
				bind:hasUserImpersonationPrivilege={draftHaveUserImpersonationPrivilege}
			/>
		</div>

		<div class="mt-4 flex shrink-0 justify-end gap-2">
			<button class="btn btn-secondary" onclick={handleClose}>Cancel</button>
			<button
				class="btn btn-primary"
				onclick={handleConfirm}
				disabled={loading || (!!groupAssignment.assignment.role && !hasChanges)}
			>
				{#if loading}
					<Loading class="size-4" />
				{:else}
					{groupAssignment.assignment.role ? 'Update' : 'Assign'}
				{/if}
			</button>
		</div>
	{/if}
</ResponsiveDialog>
