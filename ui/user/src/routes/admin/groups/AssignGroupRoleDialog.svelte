<script lang="ts">
	import { LoaderCircle, Group as GroupIcon, X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	import { groupRoleOptions } from '$lib/services/admin/constants.js';
	import { Group, Role } from '$lib/services/admin/types';
	import { profile } from '$lib/stores/index.js';
	import { getUserRoleLabel } from '$lib/utils';

	import type { GroupAssignment } from './types';

	interface Props {
		groupAssignment?: GroupAssignment;
		loading?: boolean;
		onClose: () => void;
		onConfirm: (groupAssignment: GroupAssignment) => void;
		onAuditorConfirm: (groupAssignment: GroupAssignment) => void;
		onOwnerConfirm: (groupAssignment: GroupAssignment) => void;
	}

	interface RoleOption {
		id: number;
		label: string;
	}

	let {
		groupAssignment = $bindable(),
		loading = false,
		onClose,
		onConfirm,
		onAuditorConfirm,
		onOwnerConfirm
	}: Props = $props();

	let dialog = $state<HTMLDialogElement>();

	let draftRoleId = $state(0);
	let draftAuditor = $state(false);
	let draftDescription = $state('');

	const canAssignOwner = $derived(profile.current.groups.includes(Group.OWNER));
	const canAssignAdmin = $derived(canAssignOwner || profile.current.groups.includes(Group.ADMIN));

	// Owners can assign Owner role to groups (under review per requirements)
	// Admins can assign Admin, PowerUser+, PowerUser roles (not Owner or Auditor)
	let roleOptions: RoleOption[] = $derived([
		...(canAssignOwner ? [{ label: 'Owner', id: Role.OWNER }] : []),
		...groupRoleOptions
			.filter((role) => (role.id === Role.ADMIN ? canAssignAdmin : true))
			.map((d) => ({ id: d.id, label: d.label }))
	]);

	// Check if any changes were made
	const hasChanges = $derived.by(() => {
		if (!groupAssignment?.assignment) return true;
		const currentRole = groupAssignment.assignment.role & ~Role.AUDITOR;
		const currentAuditor = (groupAssignment.assignment.role & Role.AUDITOR) !== 0;
		const currentDescription = groupAssignment.assignment.description || '';
		return (
			currentRole !== draftRoleId ||
			currentAuditor !== draftAuditor ||
			currentDescription !== draftDescription
		);
	});

	const roleDescriptionMap = $derived(
		groupRoleOptions.reduce(
			(acc, role) => {
				acc[role.id] = role.description;
				return acc;
			},
			{} as Record<number, string>
		)
	);

	const auditorReadonlyAdminRoles = [Role.BASIC, Role.POWERUSER, Role.POWERUSER_PLUS];

	$effect(() => {
		if (groupAssignment) {
			// Initialize draft values from assignment
			const role = groupAssignment.assignment.role || 0;
			draftRoleId = role & ~Role.AUDITOR;
			draftAuditor = (role & Role.AUDITOR) !== 0;
			draftDescription = groupAssignment.assignment.description || '';
			dialog?.showModal();
		} else {
			dialog?.close();
		}
	});

	function handleClose() {
		dialog?.close();
		onClose();
	}

	function handleConfirm() {
		if (!groupAssignment) return;

		const role = draftAuditor ? draftRoleId | Role.AUDITOR : draftRoleId;
		const result: GroupAssignment = {
			group: groupAssignment.group,
			assignment: {
				groupName: groupAssignment.group.name,
				role,
				description: draftDescription
			}
		};

		// Check for Owner role assignment first
		if (draftRoleId === Role.OWNER) {
			dialog?.close();
			onOwnerConfirm(result);
			groupAssignment = undefined;
			return;
		}

		// If group has no auditor flag but user is adding it
		const hadAuditor =
			groupAssignment.assignment.role && (groupAssignment.assignment.role & Role.AUDITOR) !== 0;
		if (!hadAuditor && draftAuditor && draftRoleId !== 0) {
			dialog?.close();
			onAuditorConfirm(result);
			groupAssignment = undefined;
			return;
		}

		onConfirm(result);
	}
</script>

{#snippet roleUi(role: RoleOption)}
	<label
		class="border-surface3 flex cursor-pointer gap-4 rounded-lg border p-3 hover:bg-black/2 active:bg-black/5 dark:hover:bg-white/2 dark:active:bg-white/5"
	>
		<input
			type="radio"
			value={role.id}
			bind:group={draftRoleId}
			disabled={!profile.current.groups.includes(Group.OWNER) &&
				(role.id === Role.OWNER || role.id === 0)}
		/>
		<div
			class="flex flex-col"
			class:opacity-50={!profile.current.groups.includes(Group.OWNER) &&
				(role.id === Role.OWNER || role.id === 0)}
		>
			<div class="w-28 flex-shrink-0 font-semibold whitespace-nowrap">{role.label}</div>
			<p class="text-xs text-gray-500">
				{#if role.id === Role.OWNER}
					All group members will have Owner privileges and can manage all aspects of the platform.
				{:else if role.id === Role.ADMIN}
					All group members will have Admin privileges and can manage all aspects of the platform.
				{:else if role.id === 0}
					Remove role assignment from this group.
				{:else}
					{roleDescriptionMap[role.id] || `All group members will have ${role.label} privileges.`}
				{/if}
			</p>
		</div>
	</label>
{/snippet}

<dialog bind:this={dialog} class="w-full max-w-xl overflow-visible p-4">
	{#if groupAssignment}
		<div class="mb-6 flex flex-col">
			<h3 class="default-dialog-title">
				{groupAssignment.assignment.role ? 'Update' : 'Assign'} Group Role
				<button onclick={handleClose} class="icon-button">
					<X class="size-5" />
				</button>
			</h3>

			{#if groupAssignment.assignment.role}
				<div class="dark:bg-surface1 mt-3 flex flex-col gap-1 rounded-lg bg-gray-50 p-3">
					<div class="text-md flex items-center gap-1 text-black/50 dark:text-white/50">
						<GroupIcon class="size-5" />
						<span class="font-semibold">{groupAssignment.group.name}</span>
					</div>
					<div class="text-xs text-gray-600 dark:text-gray-400">
						{getUserRoleLabel(groupAssignment.assignment.role)}
					</div>
				</div>
			{/if}
		</div>

		<div class="flex flex-col gap-2 text-sm font-light">
			{#each roleOptions as role (role.id)}
				{@render roleUi(role)}
			{/each}

			{#if profile.current.groups.includes(Group.OWNER)}
				{@const isDisabled = draftRoleId === 0}
				<label
					class={twMerge(
						'border-surface3 my-4 flex cursor-pointer gap-4 rounded-lg border p-3 hover:bg-black/2 active:bg-black/5 dark:hover:bg-white/2 dark:active:bg-black/5',
						isDisabled ? 'pointer-events-none opacity-50' : ''
					)}
					aria-disabled={isDisabled}
				>
					<input type="checkbox" bind:checked={draftAuditor} disabled={isDisabled} />
					<div class="flex flex-col">
						<div class="w-28 flex-shrink-0 font-semibold">Auditor</div>
						<p class="text-xs text-gray-500">
							{#if auditorReadonlyAdminRoles.includes(draftRoleId)}
								All group members will have read-only access to the admin system and see additional
								details such as response, request, and header information in the audit logs.
							{:else}
								All group members will gain access to additional details such as response, request,
								and header information in the audit logs.
							{/if}
						</p>
					</div>
				</label>
			{/if}

			<label class="my-2 flex flex-col gap-2">
				<span class="font-semibold">Description (Optional)</span>
				<textarea
					bind:value={draftDescription}
					class="dark:bg-surface2 dark:border-surface3 rounded-lg border p-3 text-sm"
					rows="3"
					placeholder="Add a description for this role assignment..."
				></textarea>
			</label>
		</div>
		<div class="mt-4 flex justify-end gap-2">
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
	{/if}
</dialog>
