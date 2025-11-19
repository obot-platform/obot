<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte.js';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { userRoleOptions } from '$lib/services/admin/constants.js';
	import { Group, Role, type OrgUser } from '$lib/services/admin/types';
	import { AdminService, ChatService } from '$lib/services/index.js';
	import { profile } from '$lib/stores/index.js';
	import { formatTimeAgo } from '$lib/time.js';
	import { Handshake, Info, LoaderCircle, ShieldAlert, X } from 'lucide-svelte';
	import { fade } from 'svelte/transition';
	import { getUserRoleLabel } from '$lib/utils';
	import Search from '$lib/components/Search.svelte';
	import { debounce } from 'es-toolkit';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setSortUrlParams,
		setFilterUrlParams
	} from '$lib/url.js';

	let { data } = $props();
	const { users: initialUsers } = data;

	let users = $state<OrgUser[]>(initialUsers);
	let query = $state('');
	let urlFilters = $derived(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort());

	const tableData = $derived(
		users
			.map((user) => ({
				...user,
				name: getUserDisplayName(user),
				role: getUserRoleLabel(user.role).split(','),
				roleId: user.role & ~Role.AUDITOR,
				auditor: user.role & Role.AUDITOR ? true : false
			}))
			.filter(
				(user) =>
					user.name.toLowerCase().includes(query.toLowerCase()) ||
					user.email.toLowerCase().includes(query.toLowerCase())
			)
	);

	type TableItem = (typeof tableData)[0];

	let updateRoleDialog = $state<HTMLDialogElement>();
	let updatingRole = $state<TableItem>();
	let deletingUser = $state<TableItem>();
	let confirmHandoffToUser = $state<TableItem>();
	let confirmAuditorAdditionToUser = $state<TableItem>();
	let loading = $state(false);
	let roleOptions = $derived([
		...(profile.current.groups.includes(Group.OWNER) ? [{ label: 'Owner', id: Role.OWNER }] : []),
		{ label: 'Admin', id: Role.ADMIN },
		{ label: 'Power User+', id: Role.POWERUSER_PLUS },
		{ label: 'Power User', id: Role.POWERUSER },
		{ label: 'Basic User', id: Role.BASIC }
	]);
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	function closeUpdateRoleDialog() {
		updateRoleDialog?.close();
		updatingRole = undefined;
	}

	async function updateUserRole(userID: string, role: number, refreshUsers = true) {
		loading = true;
		await AdminService.updateUserRole(userID, role);
		if (refreshUsers) {
			users = await AdminService.listUsers();
		}
		if (profile.current.id === userID) {
			// update with the role change
			profile.current = await ChatService.getProfile();
		}
		loading = false;
		closeUpdateRoleDialog();
	}

	function getUserDisplayName(user: OrgUser): string {
		let display =
			user?.displayName ??
			user?.originalUsername ??
			user?.originalEmail ??
			user?.username ??
			user?.email ??
			'Unknown User';

		if (user?.deletedAt) {
			display += ' (Deleted)';
		}

		return display;
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
	const auditorReadonlyAdminRoles = [Role.BASIC, Role.POWERUSER, Role.POWERUSER_PLUS];
</script>

<Layout title="Users">
	<div class="my-4" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			<div class="flex flex-col gap-2">
				<Search
					value={query}
					class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
					onChange={updateQuery}
					placeholder="Search by name or email..."
				/>
				<Table
					data={tableData}
					fields={['name', 'email', 'role', 'lastActiveDay']}
					filterable={['name', 'email', 'role']}
					filters={urlFilters}
					onFilter={setFilterUrlParams}
					onClearAllFilters={clearUrlParams}
					sortable={['name', 'email', 'role', 'lastActiveDay']}
					headers={[{ title: 'Last Active', property: 'lastActiveDay' }]}
					{initSort}
					onSort={setSortUrlParams}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'role'}
							<div class="flex items-center gap-1">
								{d.role}
								{#if d.explicitRole}
									<div
										use:tooltip={"This user's role is explicitly set at the system level and cannot be changed."}
									>
										<ShieldAlert class="size-5" />
									</div>
								{/if}
							</div>
						{:else if property === 'lastActiveDay'}
							{d.lastActiveDay ? formatTimeAgo(d.lastActiveDay, 'day').relativeTime : '-'}
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
											(d.groups.includes(Group.OWNER) || d.explicitRole)}
										onclick={() => {
											updatingRole = d;
											updateRoleDialog?.showModal();
										}}
									>
										Update Role
									</button>
									<button
										class="menu-button text-red-500"
										disabled={d.explicitRole ||
											(d.groups.includes(Group.OWNER) &&
												!profile.current.groups.includes(Group.OWNER))}
										onclick={() => (deletingUser = d)}
									>
										Delete User
									</button>
								</div>
							</DotDotDot>
						{/if}
					{/snippet}
				</Table>
			</div>
		</div>
	</div>
</Layout>

<Confirm
	msg={`Are you sure you want to delete user ${deletingUser?.email}?`}
	show={Boolean(deletingUser)}
	onsuccess={async () => {
		if (!deletingUser) return;
		loading = true;
		await AdminService.deleteUser(deletingUser.id);
		users = await AdminService.listUsers();
		loading = false;
		deletingUser = undefined;
	}}
	oncancel={() => (deletingUser = undefined)}
/>

<dialog bind:this={updateRoleDialog} class="w-full max-w-xl overflow-visible p-4">
	{#if updatingRole}
		{@const roleDescriptionMap = userRoleOptions.reduce(
			(acc, role) => {
				acc[role.id] = role.description;
				return acc;
			},
			{} as Record<number, string>
		)}
		<h3 class="default-dialog-title">
			Update User Role
			<button onclick={() => closeUpdateRoleDialog()} class="icon-button">
				<X class="size-5" />
			</button>
		</h3>
		<div class="m-4 flex flex-col gap-2 text-sm font-light">
			{#if updatingRole.explicitRole}
				<div class="notification-info mb-2 p-3 text-sm font-light">
					<div class="flex items-center gap-3">
						<Info class="size-6" />
						<div>This user's role is explicitly set at the system level and cannot be changed.</div>
					</div>
				</div>
			{/if}
			{#each roleOptions as role (role.id)}
				<label class="flex gap-4">
					<input
						type="radio"
						value={role.id}
						bind:group={updatingRole.roleId}
						disabled={updatingRole.explicitRole}
					/>
					<span class="flex flex-col" class:opacity-50={updatingRole.explicitRole}>
						<p class="w-28 flex-shrink-0 font-semibold">{role.label}</p>
						<p class="text-on-surface1">
							{#if role.id === Role.OWNER}
								Owners can manage all aspects of the platform and can also assign the Owner role to
								other users.
							{:else if role.id === Role.ADMIN}
								Admins can manage all aspects of the platform.
							{:else}
								{roleDescriptionMap[role.id]}
							{/if}
						</p>
					</span>
				</label>
			{/each}

			{#if profile.current.groups.includes(Group.OWNER)}
				<label class="my-4 flex gap-4">
					<input type="checkbox" bind:checked={updatingRole.auditor} />
					<span class="flex flex-col">
						<p class="w-28 flex-shrink-0 font-semibold">Auditor</p>
						{#if auditorReadonlyAdminRoles.includes(updatingRole.roleId)}
							<p class="text-on-surface1">
								Will have read-only access to the admin system and see additional details such as
								response, request, and header information in the audit logs.
							</p>
						{:else}
							<p class="text-on-surface1">
								Will gain access to additional details such as response, request, and header
								information in the audit logs.
							</p>
						{/if}
					</span>
				</label>
			{/if}
		</div>
		<div class="mt-4 flex justify-end gap-2">
			<button class="button" onclick={() => closeUpdateRoleDialog()}>Cancel</button>
			<button
				class="button-primary"
				onclick={async () => {
					if (!updatingRole) return;
					if (profile.current.isBootstrapUser?.() && updatingRole.roleId === Role.OWNER) {
						updateRoleDialog?.close();
						confirmHandoffToUser = updatingRole;
						return;
					}

					if (updatingRole.auditor) {
						updateRoleDialog?.close();
						confirmAuditorAdditionToUser = updatingRole;
						return;
					}

					updateUserRole(
						updatingRole.id,
						updatingRole.auditor ? updatingRole.roleId | Role.AUDITOR : updatingRole.roleId
					);
				}}
				disabled={loading}
			>
				{#if loading}
					<LoaderCircle class="size-4 animate-spin" />
				{:else}
					Update
				{/if}
			</button>
		</div>
	{/if}
</dialog>

<Confirm
	show={Boolean(confirmHandoffToUser)}
	{loading}
	onsuccess={async () => {
		if (!confirmHandoffToUser) return;
		await updateUserRole(
			confirmHandoffToUser.id,
			confirmHandoffToUser.auditor
				? confirmHandoffToUser.roleId | Role.AUDITOR
				: confirmHandoffToUser.roleId,
			false
		);
		await AdminService.bootstrapLogout();
		window.location.href = '/oauth2/sign_out?rd=/admin';
		confirmHandoffToUser = undefined;
	}}
	oncancel={() => (confirmHandoffToUser = undefined)}
>
	{#snippet title()}
		<div class="flex items-center justify-center gap-2">
			<Handshake class="size-6" />
			<h3 class="text-xl font-semibold">Confirm Handoff</h3>
		</div>
	{/snippet}
	{#snippet note()}
		<div class="mt-4 mb-8 flex flex-col gap-4">
			<p>
				Once you've established your first admin or owner user, the bootstrap user currently being
				used will be disabled. Upon completing this action, you'll be logged out and asked to log in
				using your auth provider.
			</p>
			<p>Are you sure you want to continue?</p>
		</div>
	{/snippet}
</Confirm>

<Confirm
	{loading}
	show={Boolean(confirmAuditorAdditionToUser)}
	onsuccess={async () => {
		if (!confirmAuditorAdditionToUser) return;
		await updateUserRole(
			confirmAuditorAdditionToUser.id,
			confirmAuditorAdditionToUser.roleId | Role.AUDITOR
		);
		confirmAuditorAdditionToUser = undefined;
	}}
	oncancel={() => (confirmAuditorAdditionToUser = undefined)}
>
	{#snippet title()}
		<div class="flex items-center justify-center gap-2">
			<h3 class="text-xl font-semibold">Confirm Auditor Role</h3>
		</div>
	{/snippet}
	{#snippet note()}
		<div class="mt-4 mb-8 flex flex-col gap-4">
			<p class="text-left">
				{#if confirmAuditorAdditionToUser && auditorReadonlyAdminRoles.includes(confirmAuditorAdditionToUser.roleId)}
					Basic user auditors will have read-only access to the admin system and can see additional
					details such as response, request, and header information in the audit logs.
				{:else}
					Auditors will gain access to additional details such as response, request, and header
					information in the audit logs.
				{/if}
			</p>
			<p>
				Are you sure you want to grant <b
					>{confirmAuditorAdditionToUser?.email || confirmAuditorAdditionToUser?.name}</b
				> this role?
			</p>
		</div>
	{/snippet}
</Confirm>

<svelte:head>
	<title>Obot | Users</title>
</svelte:head>
