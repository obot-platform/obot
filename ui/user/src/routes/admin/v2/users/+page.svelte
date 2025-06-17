<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte.js';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Select from '$lib/components/Select.svelte';
	import Table from '$lib/components/Table.svelte';
	import { Role, type OrgUser } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services/index.js';
	import { formatTimeAgo } from '$lib/time.js';
	import { LoaderCircle, ShieldAlert, Trash2, X } from 'lucide-svelte';
	import { fade } from 'svelte/transition';

	let { data } = $props();
	const { users: initialUsers } = data;

	let users = $state<OrgUser[]>(initialUsers);
	const tableData = $derived(
		users.map((user) => ({
			...user,
			role: user.role === Role.ADMIN ? 'Admin' : 'User',
			roleId: user.role,
			lastActive: user.lastActiveDay ? formatTimeAgo(user.lastActiveDay).relativeTime : '-'
		}))
	);

	let updateRoleDialog = $state<HTMLDialogElement>();
	let updatingRole = $state<(typeof tableData)[0]>();
	let deletingUser = $state<(typeof tableData)[0]>();
	let loading = $state(false);

	function closeUpdateRoleDialog() {
		updateRoleDialog?.close();
		updatingRole = undefined;
	}
</script>

<Layout>
	<div class="my-8" in:fade>
		<div class="flex flex-col gap-8">
			<div class="flex items-center justify-between">
				<h1 class="text-2xl font-semibold">Organization</h1>
			</div>

			<div class="flex flex-col gap-2">
				<h2 class="mb-2 text-lg font-semibold">Groups</h2>
				<Table data={[]} fields={[]}>
					{#snippet actions(d)}
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
					fields={['email', 'role', 'lastActive']}
					headers={[{ title: 'Last Active', property: 'lastActive' }]}
				>
					{#snippet onRenderColumn(fieldName, d)}
						{#if fieldName === 'role'}
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
						{:else}
							{d[fieldName as keyof typeof d]}
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
				<b>Users</b>: Users are restricted to only interracting with agents shared with them. They
				cannot access the Admin UI
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
					if (updatingRole) {
						loading = true;
						await AdminService.updateUserRole(updatingRole.id, updatingRole.roleId);
						users = await AdminService.listUsers();
						loading = false;
						closeUpdateRoleDialog();
					}
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

<svelte:head>
	<title>Obot | Organization</title>
</svelte:head>
