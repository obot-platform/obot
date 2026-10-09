<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { m } from '$lib/i18n';
	import { Group, Role, type GroupRoleAssignment } from '$lib/services/admin/types';
	import { AdminService, UserService, type OrgGroup } from '$lib/services/index.js';
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
	import AddGroupAssignmentDialog from './AddGroupAssignmentDialog.svelte';
	import AssignGroupRoleDialog from './AssignGroupRoleDialog.svelte';
	import ConfirmAuditorRoleDialog from './ConfirmAuditorRoleDialog.svelte';
	import ConfirmOwnerRoleDialog from './ConfirmOwnerRoleDialog.svelte';
	import CurrentAccessDialog from './CurrentAccessDialog.svelte';
	import type { GroupAssignment } from './types';
	import { debounce } from 'es-toolkit';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		groups: OrgGroup[];
		groupRoleAssignments: GroupRoleAssignment[];
	}

	let { groups: initialGroups, groupRoleAssignments: initialAssignments }: Props = $props();
	let groups = $state(untrack(() => initialGroups));
	let groupRoleAssignments = $state(untrack(() => initialAssignments));

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
	let query = $derived(page.url.searchParams.get('query') || '');
	let urlFilters = $derived(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort());

	const groupNameMap = $derived(
		groups.reduce(
			(acc, group) => {
				acc[group.id] = group.name;
				return acc;
			},
			{} as Record<string, string>
		)
	);

	const preparedGroups = $derived(
		groupRoleAssignments.map((assignment) => {
			const role = assignment.role ?? 0;
			return {
				id: assignment.groupName,
				name: groupNameMap[assignment.groupName] ?? assignment.groupName,
				assignment,
				role: role ? getUserRoleLabel(role).split(',') : [],
				roleId: getRoleId(role),
				description: assignment.description || ''
			};
		})
	);

	const filteredGroups = $derived(
		preparedGroups.filter((group) => group.name.toLowerCase().includes(query.toLowerCase()))
	);

	type TableItem = (typeof filteredGroups)[0];

	let currentAccessDialog = $state<ReturnType<typeof CurrentAccessDialog>>();
	let updatingRole = $state<TableItem>();
	let deletingGroup = $state<TableItem>();
	let showAddAssignment = $state(false);
	let showAssignGroupRoleDialog = $state(false);
	let confirmAuditorAdditionToGroup = $state<GroupAssignment>();
	let confirmOwnerGroupAssignment = $state<GroupAssignment>();
	let loading = $state(false);
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let addGroupAssignmentDialog = $state<ReturnType<typeof AddGroupAssignmentDialog>>();

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

			showAddAssignment = false;
			confirmAuditorAdditionToGroup = undefined;
			confirmOwnerGroupAssignment = undefined;

			// Refresh data, resolving names for any newly assigned group.
			groupRoleAssignments = await AdminService.listGroupRoleAssignments();

			try {
				groups = await UserService.resolveGroups(groupRoleAssignments.map((a) => a.groupName));
			} catch (error) {
				console.error('Failed to resolve group names:', error);
			}

			// Refresh user's profile if they're in the affected group
			if (profile.current.groups.includes(groupName)) {
				profile.current = await UserService.getProfile();
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

	export function openAddAssignment() {
		showAddAssignment = true;
		addGroupAssignmentDialog?.clear();
	}
</script>

<div class="mb-4" in:fade={{ duration }}>
	<div class="flex flex-col gap-8">
		<div class="flex flex-col gap-2">
			<Search
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
				value={query}
				onChange={updateQuery}
				placeholder={m.identity_access_groups_search_placeholder()}
			/>
			<div class="groups-table">
				<Table
					data={filteredGroups}
					fields={['name', 'role']}
					filterable={['name', 'role']}
					filters={urlFilters}
					onFilter={setFilterUrlParams}
					onClearAllFilters={clearUrlParams}
					sortable={['name', 'role']}
					headers={[
						{ property: 'name', title: m.core_name() },
						{ property: 'role', title: m.identity_access_col_role() }
					]}
					{initSort}
					onSort={setSortUrlParams}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'role'}
							<div class="flex items-center gap-1">
								{d.role}
							</div>
						{:else}
							{d[property as keyof typeof d]}
						{/if}
					{/snippet}

					{#snippet actions(d)}
						<DotDotDot>
							<button
								class="menu-button"
								onclick={() => {
									currentAccessDialog?.open({
										kind: 'group',
										id: d.id,
										name: d.name
									});
								}}
							>
								{m.identity_access_view_access_policies()}
							</button>
							{#if !isAdminReadonly}
								<button
									class="menu-button"
									disabled={!profile.current.groups.includes(Group.OWNER) &&
										d.roleId === Role.OWNER}
									onclick={() => {
										updatingRole = d;
										showAssignGroupRoleDialog = true;
									}}
								>
									{d.assignment ? m.identity_access_update_role() : m.identity_access_assign_role()}
								</button>
								{#if d.assignment}
									<button
										class="menu-button text-error"
										disabled={!profile.current.groups.includes(Group.OWNER) &&
											d.roleId === Role.OWNER}
										onclick={() => (deletingGroup = d)}
									>
										{m.identity_access_groups_remove_role_assignment()}
									</button>
								{/if}
							{/if}
						</DotDotDot>
					{/snippet}
				</Table>
			</div>
		</div>
	</div>
</div>

<CurrentAccessDialog bind:this={currentAccessDialog} />

<Confirm
	title={m.identity_access_groups_confirm_role_removal()}
	msg={m.identity_access_groups_remove_role_assignment_msg({ name: `${deletingGroup?.name}` })}
	show={Boolean(deletingGroup)}
	onsuccess={async () => {
		if (!deletingGroup) return;
		loading = true;
		await AdminService.deleteGroupRoleAssignment(deletingGroup.id);
		groupRoleAssignments = await AdminService.listGroupRoleAssignments();
		// Refresh user's profile if they're in the affected group
		if (profile.current.groups.includes(deletingGroup.id)) {
			profile.current = await UserService.getProfile();
		}
		loading = false;
		deletingGroup = undefined;
	}}
	oncancel={() => (deletingGroup = undefined)}
>
	{#snippet note()}
		{m.identity_access_groups_remove_role_assignment_note()}
	{/snippet}
</Confirm>

<AddGroupAssignmentDialog
	bind:this={addGroupAssignmentDialog}
	open={showAddAssignment}
	{groupRoleMap}
	{loading}
	onClose={() => (showAddAssignment = false)}
	onConfirm={updateGroupRole}
	onOwnerConfirm={(groupAssignment) => {
		confirmOwnerGroupAssignment = groupAssignment;
	}}
	onAuditorConfirm={(groupAssignment) => {
		confirmAuditorAdditionToGroup = groupAssignment;
	}}
/>

<AssignGroupRoleDialog
	open={showAssignGroupRoleDialog}
	groupAssignment={updatingRole
		? {
				group: { id: updatingRole.id, name: updatingRole.name },
				assignment: updatingRole.assignment || { groupName: updatingRole.id, role: 0 }
			}
		: undefined}
	{loading}
	onClose={() => {
		showAssignGroupRoleDialog = false;
	}}
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
	oncancel={() => {
		// return to previous dialog
		confirmAuditorAdditionToGroup = undefined;
		if (updatingRole) {
			showAssignGroupRoleDialog = true;
		} else {
			showAddAssignment = true;
		}
	}}
/>

<ConfirmOwnerRoleDialog
	bind:groupAssignment={confirmOwnerGroupAssignment}
	{loading}
	onsuccess={(groupAssignment) => {
		updateGroupRole(groupAssignment);
		confirmOwnerGroupAssignment = undefined;
		confirmAuditorAdditionToGroup = undefined;
	}}
	oncancel={() => {
		confirmOwnerGroupAssignment = undefined;
		if (updatingRole) {
			showAssignGroupRoleDialog = true;
		} else {
			showAddAssignment = true;
		}
	}}
/>

<style>
	.groups-table :global(td) {
		position: relative;
	}
</style>
