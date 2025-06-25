<script lang="ts">
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService } from '$lib/services';
	import { Role, type MCPCatalog, type OrgUser } from '$lib/services/admin/types';
	import { LoaderCircle, Plus, Trash2 } from 'lucide-svelte';
	import { onMount, type Snippet } from 'svelte';
	import { fly } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Table from '../Table.svelte';
	import SearchUsers from './SearchUsers.svelte';
	import Confirm from '../Confirm.svelte';
	import { goto } from '$app/navigation';

	interface Props {
		topContent?: Snippet;
		mcpCatalog?: MCPCatalog;
		onCreate?: (catalog: MCPCatalog) => void;
	}

	let { topContent, mcpCatalog: initialMcpCatalog, onCreate }: Props = $props();
	const duration = PAGE_TRANSITION_DURATION;
	let mcpCatalog = $state(
		initialMcpCatalog ??
			({
				displayName: '',
				sourceURLs: [],
				allowedUserIDs: [],
				id: ''
			} satisfies MCPCatalog)
	);

	let saving = $state<boolean | undefined>();
	let loadingUsers = $state<Promise<OrgUser[]>>();

	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();

	let deletingUserGroup = $state<{ id: string; email: string }>();
	let deletingCatalog = $state(false);

	onMount(async () => {
		loadingUsers = AdminService.listUsers();
	});

	function convertUsersToTableData(userIds: string[], users: OrgUser[]) {
		const userMap = new Map(users?.map((user) => [user.id, user]));
		return (
			userIds
				.map((id) => {
					if (id === '*') {
						return {
							id: '*',
							username: 'everyone',
							email: 'Everyone',
							role: 'User',
							iconURL: '',
							created: new Date().toISOString(),
							explicitAdmin: false,
							type: 'Group'
						};
					}

					const user = userMap.get(id);
					if (!user) {
						return undefined;
					}

					return {
						...user,
						role: user.role === Role.ADMIN ? 'Admin' : 'User',
						type: 'User'
					};
				})
				.filter((user) => user !== undefined) ?? []
		);
	}

	function validate(catalog: typeof mcpCatalog) {
		if (!catalog) return false;

		return catalog.displayName.length > 0 && catalog.allowedUserIDs.length > 0;
	}
</script>

<div
	class="flex flex-col gap-8"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div class="flex flex-col gap-8" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		{#if topContent}
			{@render topContent()}
		{/if}
		{#if mcpCatalog.id}
			<div class="flex w-full items-center justify-between gap-4">
				<h1 class="flex items-center gap-4 text-2xl font-semibold">
					{mcpCatalog.displayName}
				</h1>
				<button
					class="button-destructive flex items-center gap-1 text-xs font-normal"
					use:tooltip={'Delete Catalog'}
					onclick={() => {
						deletingCatalog = true;
					}}
				>
					<Trash2 class="size-4" />
				</button>
			</div>
		{:else}
			<h1 class="text-2xl font-semibold">Create MCP Catalog</h1>
		{/if}

		{#if !mcpCatalog.id}
			<div
				class="dark:bg-surface2 dark:border-surface3 rounded-lg border border-transparent bg-white p-4"
			>
				<div class="flex flex-col gap-6">
					<div class="flex flex-col gap-2">
						<label for="mcp-catalog-name" class="flex-1 text-sm font-light capitalize">
							Name
						</label>
						<input
							id="mcp-catalog-name"
							bind:value={mcpCatalog.displayName}
							class="text-input-filled mt-0.5"
						/>
					</div>
				</div>
			</div>
		{/if}

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">User & Groups</h2>
				<div class="relative flex items-center gap-4">
					{#await loadingUsers}
						<button class="button-primary flex items-center gap-1 text-sm" disabled>
							<Plus class="size-4" /> Add User/Group
						</button>
					{:then _users}
						<button
							class="button-primary flex items-center gap-1 text-sm"
							onclick={() => {
								addUserGroupDialog?.open();
							}}
						>
							<Plus class="size-4" /> Add User/Group
						</button>
					{/await}
				</div>
			</div>
			{#await loadingUsers}
				<div class="my-2 flex items-center justify-center">
					<LoaderCircle class="size-6 animate-spin" />
				</div>
			{:then users}
				{@const userData = convertUsersToTableData(mcpCatalog?.allowedUserIDs ?? [], users ?? [])}
				<Table
					data={userData}
					fields={['email', 'type', 'role']}
					noDataMessage={'No users or groups added.'}
				>
					{#snippet actions(d)}
						<button
							class="icon-button hover:text-red-500"
							onclick={() => {
								deletingUserGroup = d;
							}}
							use:tooltip={'Delete User/Group'}
						>
							<Trash2 class="size-4" />
						</button>
					{/snippet}
				</Table>
			{/await}
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">MCP Servers</h2>
				<div class="relative flex items-center gap-4">
					<button
						class="button-primary flex items-center gap-1 text-sm"
						onclick={() => {
							// TODO:
						}}
					>
						<Plus class="size-4" /> Add MCP Server
					</button>
				</div>
			</div>
			<Table data={[]} fields={[]} noDataMessage={'No MCP servers.'}>
				{#snippet actions(d)}
					<button
						class="icon-button hover:text-red-500"
						onclick={() => {
							// TODO:
						}}
						use:tooltip={'Remove MCP Server'}
					>
						<Trash2 class="size-4" />
					</button>
				{/snippet}
			</Table>
		</div>
	</div>
	<div
		class="bg-surface1 sticky bottom-0 left-0 flex w-full justify-end gap-2 py-4 text-gray-400 dark:bg-black dark:text-gray-600"
		out:fly={{ x: -100, duration }}
		in:fly={{ x: -100 }}
	>
		{#if mcpCatalog.id}
			{#if saving === true}
				<div class="flex items-center justify-center font-light">
					<LoaderCircle class="size-6 animate-spin" /> Saving...
				</div>
			{:else if saving === false}
				<div class="flex items-center justify-center font-light">Saved.</div>
			{/if}
		{:else}
			<div class="flex w-full justify-end gap-2">
				<button class="button">Cancel</button>
				<button
					class="button-primary disabled:opacity-75"
					disabled={!validate(mcpCatalog)}
					onclick={async () => {
						saving = true;
						const response = await AdminService.createMCPCatalog(mcpCatalog);
						mcpCatalog = response;
						onCreate?.(mcpCatalog);
						saving = false;
					}}
				>
					{#if saving}
						<LoaderCircle class="size-4 animate-spin" />
					{:else}
						Create Catalog
					{/if}
				</button>
			</div>
		{/if}
	</div>
</div>

<SearchUsers
	bind:this={addUserGroupDialog}
	filterIds={mcpCatalog?.allowedUserIDs}
	onAdd={async (users) => {
		saving = true;
		const existingEmails = new Set(mcpCatalog.allowedUserIDs ?? []);
		const newUsers = users.filter((user) => !existingEmails.has(user.id));
		mcpCatalog.allowedUserIDs = [
			...(mcpCatalog?.allowedUserIDs ?? []),
			...newUsers.map((user) => user.id)
		];

		if (mcpCatalog.id) {
			const response = await AdminService.updateMCPCatalog(mcpCatalog.id, mcpCatalog);
			mcpCatalog = response;
		}
		saving = false;
	}}
/>

<Confirm
	msg={`Delete ${deletingUserGroup?.email}?`}
	show={Boolean(deletingUserGroup)}
	onsuccess={async () => {
		saving = true;
		mcpCatalog.allowedUserIDs = mcpCatalog.allowedUserIDs.filter(
			(id) => id !== deletingUserGroup?.id
		);
		const response = await AdminService.updateMCPCatalog(mcpCatalog.id, mcpCatalog);
		mcpCatalog = response;
		deletingUserGroup = undefined;
		saving = false;
	}}
	oncancel={() => (deletingUserGroup = undefined)}
/>

<Confirm
	msg="Are you sure you want to delete this catalog?"
	show={deletingCatalog}
	onsuccess={async () => {
		saving = true;
		await AdminService.deleteMCPCatalog(mcpCatalog.id);
		goto('/v2/admin/access-control');
	}}
	oncancel={() => (deletingCatalog = false)}
/>
