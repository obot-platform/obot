<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte.js';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Select from '$lib/components/Select.svelte';
	import Table from '$lib/components/Table.svelte';
	import { BOOTSTRAP_USER_ID, PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { Role, type OrgUser } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services/index.js';
	import { profile } from '$lib/stores/index.js';
	import { formatTimeAgo } from '$lib/time.js';
	import { Handshake, LoaderCircle, ShieldAlert, Trash2, X } from 'lucide-svelte';
	import { fade } from 'svelte/transition';

	let { data } = $props();
	const { users: initialUsers } = data;

	let users = $state<OrgUser[]>(initialUsers);
	const tableData = $derived(
		users.map((user) => ({
			...user,
			name: getUserDisplayName(user),
			role: user.role === Role.ADMIN ? 'Admin' : 'User',
			roleId: user.role
		}))
	);

	type TableItem = (typeof tableData)[0];

	let updateRoleDialog = $state<HTMLDialogElement>();
	let updatingRole = $state<TableItem>();
	let deletingUser = $state<TableItem>();
	let confirmHandoffToUser = $state<TableItem>();
	let loading = $state(false);

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

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div class="my-4" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			<div class="flex items-center justify-between">
				<h1 class="text-2xl font-semibold">Users</h1>
			</div>

			<div class="flex flex-col gap-2">
				<h2 class="mb-2 text-lg font-semibold">Groups</h2>
				<Table data={[]} fields={[]}>
					{#snippet actions()}
						<button class="icon-button hover:text-red-500" onclick={() => {}}>
							<Trash2 class="size-4" />
						</button>
					{/snippet}
				</Table>
			</div>

			<div class="flex flex-col gap-2">
				<h2 class="mb-2 text-lg font-semibold">Users</h2>
				<Table
					data={tableData}
					fields={['name', 'email', 'role', 'lastActiveDay']}
					sortable={['name', 'email', 'role', 'lastActiveDay']}
					headers={[{ title: 'Last Active', property: 'lastActiveDay' }]}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'role'}
							<div class="flex items-center gap-1">
								{d.role}
								{#if d.explicitAdmin}
									<div
										use:tooltip={'This user is explicitly set as an admin at the system level and their role cannot be changed.'}
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
						<DotDotDot>
							<div class="default-dialog flex min-w-max flex-col p-2">
								<button
									class="menu-button"
									disabled={d.explicitAdmin}
									onclick={() => {
										updatingRole = d;
										updateRoleDialog?.showModal();
									}}
								>
									Update Role
								</button>
								<button
									class="menu-button text-red-500"
									disabled={d.explicitAdmin}
									onclick={() => (deletingUser = d)}
								>
									Delete User
								</button>
							</div>
						</DotDotDot>
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

<dialog bind:this={updateRoleDialog} class="w-full max-w-md overflow-visible p-4">
	{#if updatingRole}
		<h3 class="default-dialog-title">
			Update User Role
			<button onclick={() => closeUpdateRoleDialog()} class="icon-button">
				<X class="size-5" />
			</button>
		</h3>
		<div class="my-4 flex flex-col gap-2 text-sm text-gray-500">
			<p><b>Admin</b>: Admins can manage all aspects of the platform.</p>
			<p>
				<b>Users</b>: Users are restricted to only interracting with projects that were shared with
				them. They cannot access the Admin UI.
			</p>
		</div>
		<div>
			<Select
				class="bg-surface1 shadow-inner"
				options={[
					{ label: 'Admin', id: Role.ADMIN },
					{ label: 'User', id: Role.USER }
				]}
				selected={updatingRole.roleId}
				onSelect={(option) => {
					if (updatingRole) {
						updatingRole.roleId = option.id as number;
					}
				}}
			/>
		</div>
		<div class="mt-4 flex justify-end gap-2">
			<button class="button" onclick={() => closeUpdateRoleDialog()}>Cancel</button>
			<button
				class="button-primary"
				onclick={async () => {
					if (!updatingRole) return;
					if (
						profile.current.username === BOOTSTRAP_USER_ID &&
						updatingRole.roleId === Role.ADMIN
					) {
						updateRoleDialog?.close();
						confirmHandoffToUser = updatingRole;
						return;
					}

					updateUserRole(updatingRole.id, updatingRole.roleId);
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
		await updateUserRole(confirmHandoffToUser.id, Role.ADMIN, false);
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
				Once you've established your first admin user, the bootstrap user currently being used will
				be disabled. Upon completing this action, you'll be logged out and asked to log in using
				your auth provider.
			</p>
			<p>Are you sure you want to continue?</p>
		</div>
	{/snippet}
</Confirm>

<svelte:head>
	<title>Obot | Users</title>
</svelte:head>
