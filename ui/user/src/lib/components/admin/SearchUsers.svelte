<script lang="ts">
	import { AdminService } from '$lib/services';
	import { Role, type OrgUser } from '$lib/services/admin/types';
	import { Check, LoaderCircle, User, Users } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import Search from '../Search.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';

	interface Props {
		onAdd: (users: OrgUser[]) => void;
		filterIds?: string[];
	}

	let addUserGroupDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let fetchingUsers = $state<Promise<OrgUser[]>>();
	let searchUsers = $state('');
	let selectedUsers = $state<OrgUser[]>([]);
	let selectedUsersMap = $derived(new Set(selectedUsers.map((user) => user.id)));

	export function open() {
		addUserGroupDialog?.open();
	}

	function onOpen() {
		fetchingUsers = AdminService.listUsers();
	}

	function onClose() {
		searchUsers = '';
		selectedUsers = [];
	}

	let { onAdd, filterIds }: Props = $props();

	function getFilteredUsers(users?: OrgUser[]) {
		if (!users) {
			return [];
		}

		const withEveryone = [
			{
				id: '*',
				username: 'everyone',
				email: 'Everyone',
				role: 10,
				iconURL: '',
				created: new Date().toISOString(),
				explicitAdmin: false
			} satisfies OrgUser,
			...users
		];

		const filterIdSet = new Set(filterIds);
		const filteredIds = withEveryone.filter((user) => !filterIdSet.has(user.id));

		return searchUsers.length > 0
			? (filteredIds?.filter(
					(user) =>
						user.email.toLowerCase().includes(searchUsers.toLowerCase()) ||
						user.username.toLowerCase().includes(searchUsers.toLowerCase())
				) ?? [])
			: (filteredIds ?? []);
	}
</script>

<ResponsiveDialog
	bind:this={addUserGroupDialog}
	{onClose}
	{onOpen}
	title="Add User/Group"
	class="h-full w-full overflow-visible p-0 md:h-[500px] md:max-w-md"
	classes={{ header: 'p-4 md:pb-0' }}
>
	<div class="default-scrollbar-thin flex grow flex-col gap-4 overflow-y-scroll pt-1">
		{#await fetchingUsers}
			<div class="flex grow items-center justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:then users}
			{@const filteredUsers = getFilteredUsers(users)}
			<div class="px-4">
				<Search
					class="dark:bg-surface1 dark:border-surface3 shadow-inner dark:border"
					onChange={(val) => (searchUsers = val)}
					placeholder="Search by name or email..."
				/>
			</div>
			<div class="flex flex-col">
				{#each filteredUsers ?? [] as user (user.id)}
					<button
						class={twMerge(
							'dark:hover:bg-surface1 hover:bg-surface2 flex items-center gap-2 px-4 py-2 text-left',
							selectedUsersMap.has(user.id) && 'dark:bg-gray-920 bg-gray-50'
						)}
						onclick={() => {
							if (selectedUsersMap.has(user.id)) {
								const index = selectedUsers.findIndex((u) => u.id === user.id);
								if (index !== -1) {
									selectedUsers.splice(index, 1);
								}
							} else {
								selectedUsers.push(user);
								selectedUsersMap.add(user.id);
							}
						}}
					>
						{#if user.iconURL}
							<img src={user.iconURL} alt={user.username} class="size-10 rounded-full" />
						{:else}
							<Users class="size-10 rounded-full p-2" />
						{/if}
						<div class="flex grow flex-col">
							<p>{user.email}</p>
							<p class="font-light text-gray-400 dark:text-gray-600">
								{user.username === 'everyone'
									? 'Group'
									: user.role === Role.ADMIN
										? 'Admin'
										: 'User'}
							</p>
						</div>
						<div class="flex items-center justify-center">
							{#if selectedUsersMap.has(user.id)}
								<Check class="size-6 text-blue-500" />
							{/if}
						</div>
					</button>
				{/each}
			</div>
		{/await}
	</div>
	<div class="flex w-full flex-col justify-between gap-4 p-4 md:flex-row">
		<div class="flex items-center gap-1 font-light">
			{#if selectedUsers.length > 0}
				{#if selectedUsers.length === 1}
					<User class="size-4" />
				{:else}
					<Users class="size-4" />
				{/if}
				{selectedUsers.length} Selected
			{/if}
		</div>
		<div class="flex items-center gap-4">
			<button class="button w-full md:w-fit" onclick={() => addUserGroupDialog?.close()}>
				Cancel
			</button>
			<button
				class="button-primary w-full md:w-fit"
				onclick={() => {
					onAdd(selectedUsers);
					addUserGroupDialog?.close();
				}}
			>
				Confirm
			</button>
		</div>
	</div>
</ResponsiveDialog>
