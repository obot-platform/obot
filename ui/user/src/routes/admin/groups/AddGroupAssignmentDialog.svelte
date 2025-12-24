<script lang="ts">
	import { debounce } from 'es-toolkit';
	import { LoaderCircle, Group as GroupIcon, ChevronLeft } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import { Role, type OrgGroup, type GroupRoleAssignment } from '$lib/services/admin/types';
	import { responsive } from '$lib/stores/index.js';
	import { getUserRoleLabel } from '$lib/utils';
	import GroupRoleForm from './GroupRoleForm.svelte';
	import type { GroupAssignment } from './types';
	import Search from '$lib/components/Search.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';

	interface Props {
		open?: boolean;
		groups: OrgGroup[];
		groupRoleMap: Record<string, GroupRoleAssignment>;
		loading?: boolean;
		onClose: () => void;
		onConfirm: (groupAssignment: GroupAssignment) => void;
		onAuditorConfirm: (groupAssignment: GroupAssignment) => void;
		onOwnerConfirm: (groupAssignment: GroupAssignment) => void;
	}

	function hasAuditorFlag(role: number): boolean {
		return (role & Role.AUDITOR) !== 0;
	}

	function addAuditorFlag(role: number): number {
		return role | Role.AUDITOR;
	}

	let {
		open = $bindable(),
		groups,
		groupRoleMap,
		loading = false,
		onClose,
		onConfirm,
		onAuditorConfirm,
		onOwnerConfirm
	}: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let searchQuery = $state('');
	let selectedGroup = $state<OrgGroup | undefined>();
	let draftRoleId = $state(0);
	let draftHaveAuditorPrivilege = $state(false);

	let isSmallScreen = $derived(responsive.isMobile);

	// Filter groups by search query
	const availableGroups = $derived(
		groups.filter((group) => group.name.toLowerCase().includes(searchQuery.toLowerCase()))
	);

	function resetForm() {
		searchQuery = '';
		selectedGroup = undefined;
		draftRoleId = 0;
		draftHaveAuditorPrivilege = false;
	}

	$effect(() => {
		if (open) {
			resetForm();
			dialog?.open();
		}
	});

	function handleClose() {
		open = false;
		onClose();
	}

	function handleGroupSelect(group: OrgGroup) {
		selectedGroup = group;
		// Load existing assignment if available
		const existingAssignment = groupRoleMap[group.name];
		if (existingAssignment) {
			const role = existingAssignment.role || 0;
			draftRoleId = role & ~Role.AUDITOR;
			draftHaveAuditorPrivilege = hasAuditorFlag(role);
		} else {
			draftRoleId = 0;
			draftHaveAuditorPrivilege = false;
		}
	}

	function handleBack() {
		resetForm();
	}

	function handleConfirm() {
		if (!selectedGroup) return;

		const role = draftHaveAuditorPrivilege ? addAuditorFlag(draftRoleId) : draftRoleId;
		const result: GroupAssignment = {
			group: selectedGroup,
			assignment: {
				groupName: selectedGroup.name,
				role
			}
		};

		// Check if group already had auditor privilege
		const existingAssignment = groupRoleMap[selectedGroup.name];
		const hadAuditorBefore = existingAssignment
			? hasAuditorFlag(existingAssignment.role || 0)
			: false;

		// Auditor changed - show auditor confirmation only if they didn't have it before
		if (draftHaveAuditorPrivilege && !hadAuditorBefore && draftRoleId !== 0) {
			onAuditorConfirm(result);
			return;
		}

		// Changing to owner role - show owner confirmation
		if (draftRoleId === Role.OWNER) {
			onOwnerConfirm(result);
			return;
		}

		onConfirm(result);
	}

	const updateSearch = debounce((value: string) => {
		searchQuery = value;
	}, 100);
</script>

{#snippet groupList()}
	<div class="flex flex-col gap-4 overflow-y-auto pr-2">
		<Search value={searchQuery} onChange={updateSearch} />

		<div class="flex flex-col gap-2">
			{#if availableGroups.length === 0}
				<p class="text-on-surface1 py-8 text-center text-sm">
					{searchQuery ? 'No groups found matching your search.' : 'No groups available.'}
				</p>
			{:else}
				{#each availableGroups as group (group.id)}
					{@const hasAssignment = !!groupRoleMap[group.name]}
					{@const assignedRole = groupRoleMap[group.name]?.role}
					<button
						onclick={() => handleGroupSelect(group)}
						class={twMerge(
							'border-surface3 hover:bg-background/5 flex items-center gap-3 rounded-lg border p-3 text-left transition-colors',
							selectedGroup?.id === group.id && 'bg-primary/10 border-primary'
						)}
					>
						<div class="flex flex-1 items-center gap-3">
							{#if group.iconURL}
								<img src={group.iconURL} alt={group.name} class="size-8 rounded-full" />
							{:else}
								<div
									class="dark:bg-surface3 flex size-8 items-center justify-center rounded-full bg-gray-200"
								>
									<GroupIcon class="size-4" />
								</div>
							{/if}
							<div class="flex flex-1 flex-col">
								<span class="font-medium">{group.name}</span>
								{#if hasAssignment && assignedRole}
									<span class="text-on-surface1 text-xs">{getUserRoleLabel(assignedRole)}</span>
								{/if}
							</div>
						</div>
					</button>
				{/each}
			{/if}
		</div>
	</div>
{/snippet}

{#snippet roleForm()}
	<div class="flex flex-col gap-4 overflow-y-auto pr-2">
		{#if selectedGroup}
			<div class="dark:bg-surface1 flex flex-col gap-1 rounded-lg bg-gray-50 p-3">
				<div class="text-md flex items-center gap-2">
					{#if selectedGroup.iconURL}
						<img src={selectedGroup.iconURL} alt={selectedGroup.name} class="size-6 rounded-full" />
					{:else}
						<GroupIcon class="size-5" />
					{/if}
					<span class="font-semibold">{selectedGroup.name}</span>
				</div>
				<div class="text-on-surface1 text-xs">
					{#if groupRoleMap[selectedGroup.name]}
						Update the role for this group
					{:else}
						Select a role to assign to this group
					{/if}
				</div>
			</div>

			<GroupRoleForm
				bind:roleId={draftRoleId}
				bind:hasAuditorPrivilege={draftHaveAuditorPrivilege}
			/>
		{:else}
			<div class="text-on-surface1 flex h-full items-center justify-center py-12 text-sm">
				Select a group to assign a role
			</div>
		{/if}
	</div>
{/snippet}

{#if open}
	<ResponsiveDialog
		bind:this={dialog}
		onClose={handleClose}
		class={twMerge(
			'flex max-h-[90svh] max-w-[94svw] flex-col overflow-visible md:min-h-[768px]',
			!isSmallScreen ? 'w-full max-w-4xl' : 'w-full'
		)}
		classes={{ content: 'p-4 overflow-hidden', header: 'mb-4 flex', title: 'flex flex-1' }}
	>
		{#snippet titleContent()}
			{#if isSmallScreen && selectedGroup}
				<button
					onclick={handleBack}
					class="icon-button mr-2 -ml-2 flex-shrink-0"
					aria-label="Go back"
				>
					<ChevronLeft class="size-6" />
				</button>
			{:else if isSmallScreen}
				<div class="size-11"></div>
			{/if}

			<span class="flex-1 text-center text-lg font-semibold md:text-start md:text-xl">
				{#if selectedGroup && groupRoleMap[selectedGroup.name]}
					Update Group Role
				{:else}
					Assign Group Role
				{/if}
			</span>
		{/snippet}

		{#if !isSmallScreen}
			<!-- Large screen: two-column layout -->
			<div class="grid flex-1 grid-cols-2 gap-8 overflow-hidden">
				<div class="flex flex-col overflow-hidden">
					<h4 class="mb-4 flex-shrink-0 text-sm font-semibold">Select Group</h4>
					{@render groupList()}
				</div>
				<div class="flex flex-col overflow-hidden">
					<h4 class="mb-4 flex-shrink-0 text-sm font-semibold">Assign Role</h4>
					{@render roleForm()}
				</div>
			</div>
		{:else}
			<!-- Small screen: single column with conditional rendering -->
			{#if !selectedGroup}
				<div class="flex flex-1 flex-col overflow-hidden">
					<h4 class="mb-4 flex-shrink-0 text-sm font-semibold">Select Group</h4>
					{@render groupList()}
				</div>
			{:else}
				<div class="flex flex-1 flex-col overflow-hidden">
					{@render roleForm()}
				</div>
			{/if}
		{/if}

		<div class="mt-6 flex flex-shrink-0 flex-col justify-end gap-2 md:flex-row">
			<button class="button" onclick={handleClose}>Cancel</button>
			<button
				class="button-primary"
				onclick={handleConfirm}
				disabled={loading || !selectedGroup || draftRoleId === 0}
			>
				{#if loading}
					<LoaderCircle class="size-4 animate-spin" />
				{:else if selectedGroup && groupRoleMap[selectedGroup.name]}
					Update Role
				{:else}
					Assign Role
				{/if}
			</button>
		</div>
	</ResponsiveDialog>
{/if}
