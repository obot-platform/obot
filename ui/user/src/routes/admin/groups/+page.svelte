<script lang="ts">
	import { debounce } from 'es-toolkit';
	import { fade } from 'svelte/transition';
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { AdminService, ChatService } from '$lib/services/index.js';
	import { Group, Role, type GroupRoleAssignment } from '$lib/services/admin/types';
	import { profile } from '$lib/stores/index.js';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setSortUrlParams,
		setFilterUrlParams,
		replaceState
	} from '$lib/url.js';
	import { getUserRoleLabel } from '$lib/utils';

	import AssignGroupRoleDialog from './AssignGroupRoleDialog.svelte';
	import ConfirmAuditorRoleDialog from './ConfirmAuditorRoleDialog.svelte';
	import ConfirmOwnerRoleDialog from './ConfirmOwnerRoleDialog.svelte';
	import type { GroupAssignment } from './types';

	let { data } = $props();
	let { groups, groupRoleAssignments } = $derived(data);

	function getRoleId(role: number): number {
		return role & ~Role.AUDITOR;
	}

	// Create a map for quick role lookups
	const groupRoleMap = $derived(
		groupRoleAssignments.reduce(
			(acc, assignment) => {
				acc[assignment.groupName] = assignment;
				return acc;
			},
			{} as Record<string, GroupRoleAssignment>
		)
	);
	let query = $state(page.url.searchParams.get('query') || '');
	let urlFilters = $derived(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort());

	const preparedGroups = $derived(
		groups.map((group) => {
			const assignment = groupRoleMap[group.name];
			const role = assignment?.role ?? 0;
			return {
				...group,
				assignment,
				role: role ? getUserRoleLabel(role).split(',') : ['No Role'],
				roleId: getRoleId(role),
				description: assignment?.description || ''
			};
		})
	);

	const filteredGroups = $derived(
		preparedGroups.filter((group) => group.name.toLowerCase().includes(query.toLowerCase()))
	);

	type TableItem = (typeof filteredGroups)[0];

	let updatingRole = $state<TableItem>();
	let deletingGroup = $state<TableItem>();
	let confirmAuditorAdditionToGroup = $state<GroupAssignment>();
	let confirmOwnerGroupAssignment = $state<GroupAssignment>();
	let loading = $state(false);
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	async function updateGroupRole(data: GroupAssignment) {
		const { assignment } = data;
		loading = true;
		try {
			const { role, groupName } = assignment;

			if (role === 0) {
				// Delete the role assignment
				await AdminService.deleteGroupRoleAssignment(groupName);
			} else if (groupRoleMap[groupName]) {
				// Update existing assignment
				await AdminService.updateGroupRoleAssignment(groupName, assignment);
			} else {
				// Create new assignment
				await AdminService.createGroupRoleAssignment(assignment);
			}

			// Refresh data
			groupRoleAssignments = await AdminService.listGroupRoleAssignments();

			// Refresh user's profile if they're in the affected group
			if (profile.current.groups.includes(groupName)) {
				profile.current = await ChatService.getProfile();
			}
		} catch (error) {
			console.error('Failed to update group role:', error);
		}
		loading = false;
		updatingRole = undefined;
	}

	const updateQuery = debounce((value: string) => {
		query = value;

		if (value) {
			page.url.searchParams.set('query', value);
		} else {
			page.url.searchParams.delete('query');
		}

		replaceState(page.url, { query });
	}, 100);

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div class="my-4" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			<div class="flex items-center justify-between">
				<h1 class="text-2xl font-semibold">Group Role Assignments</h1>
			</div>

			<div class="flex flex-col gap-2">
				<Search
					value={query}
					class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
					onChange={updateQuery}
					placeholder="Search by group name..."
				/>
				<div class="groups-table">
					<Table
						data={filteredGroups}
						fields={['name', 'role', 'description']}
						filterable={['name', 'role', 'description']}
						filters={urlFilters}
						onFilter={setFilterUrlParams}
						onClearAllFilters={clearUrlParams}
						sortable={['name', 'role']}
						headers={[{ property: 'name', title: 'Name' }]}
						{initSort}
						onSort={setSortUrlParams}
					>
						{#snippet onRenderColumn(property, d)}
							{#if property === 'role'}
								<div class="flex items-center gap-1">
									{d.role}
								</div>
							{:else if property === 'description'}
								<div class="absolute inset-0 flex items-center [padding:inherit]">
									<p class="max-w-full truncate opacity-50">{d.description || '-'}</p>
								</div>
							{:else}
								{d[property as keyof typeof d]}
							{/if}
						{/snippet}

						{#snippet actions(d)}
							{#if !isAdminReadonly}
								<DotDotDot>
									<div class="default-dialog flex min-w-max flex-col p-2">
										<button
											class="menu-button"
											disabled={!profile.current.groups.includes(Group.OWNER) &&
												d.roleId === Role.OWNER}
											onclick={() => (updatingRole = d)}
										>
											{d.assignment ? 'Update Role' : 'Assign Role'}
										</button>
										{#if d.assignment}
											<button
												class="menu-button text-red-500"
												disabled={!profile.current.groups.includes(Group.OWNER) &&
													d.roleId === Role.OWNER}
												onclick={() => (deletingGroup = d)}
											>
												Remove Role Assignment
											</button>
										{/if}
									</div>
								</DotDotDot>
							{/if}
						{/snippet}
					</Table>
				</div>
			</div>
		</div>
	</div>
</Layout>

<Confirm
	msg={`Are you sure you want to remove the role assignment for group "${deletingGroup?.name}"?`}
	show={Boolean(deletingGroup)}
	onsuccess={async () => {
		if (!deletingGroup) return;
		loading = true;
		await AdminService.deleteGroupRoleAssignment(deletingGroup.name);
		groupRoleAssignments = await AdminService.listGroupRoleAssignments();
		// Refresh user's profile if they're in the affected group
		if (profile.current.groups.includes(deletingGroup.name)) {
			profile.current = await ChatService.getProfile();
		}
		loading = false;
		deletingGroup = undefined;
	}}
	oncancel={() => (deletingGroup = undefined)}
/>

<AssignGroupRoleDialog
	groupAssignment={updatingRole
		? {
				group: { id: updatingRole.id, name: updatingRole.name, iconURL: updatingRole.iconURL },
				assignment: updatingRole.assignment || { groupName: updatingRole.name, role: 0 }
			}
		: undefined}
	{loading}
	onClose={() => (updatingRole = undefined)}
	onConfirm={updateGroupRole}
	onOwnerConfirm={(groupAssignment) => {
		confirmOwnerGroupAssignment = groupAssignment;
	}}
	onAuditorConfirm={(groupAssignment) => {
		confirmAuditorAdditionToGroup = groupAssignment;
	}}
/>

<ConfirmAuditorRoleDialog
	bind:groupAssignment={confirmAuditorAdditionToGroup}
	{loading}
	onsuccess={(groupAssignment) => {
		// Check if also changing to owner role
		const originalRoleId = getRoleId(updatingRole?.assignment?.role || 0);
		const newRoleId = getRoleId(groupAssignment.assignment.role);

		if (newRoleId === Role.OWNER && originalRoleId !== Role.OWNER) {
			confirmOwnerGroupAssignment = groupAssignment;
			confirmAuditorAdditionToGroup = undefined;
			return;
		}

		updateGroupRole(groupAssignment);
		confirmAuditorAdditionToGroup = undefined;
		updatingRole = undefined;
	}}
	oncancel={() => (confirmAuditorAdditionToGroup = undefined)}
/>

<ConfirmOwnerRoleDialog
	bind:groupAssignment={confirmOwnerGroupAssignment}
	{loading}
	onsuccess={(groupAssignment) => {
		updateGroupRole(groupAssignment);
		confirmOwnerGroupAssignment = undefined;
		confirmAuditorAdditionToGroup = undefined;
	}}
	oncancel={() => (confirmOwnerGroupAssignment = undefined)}
/>

<svelte:head>
	<title>Obot | Groups</title>
</svelte:head>

<style>
	.groups-table :global(td) {
		position: relative;
	}
</style>
