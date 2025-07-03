<script lang="ts">
	import { AdminService } from '$lib/services';
	import { Role, type OrgGroup, type OrgUser } from '$lib/services/admin/types';
	import { Check, LoaderCircle, User, Users } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import Search from '../Search.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';

	interface Props {
		onAdd: (users: OrgUser[], groups: OrgGroup[]) => void;
		filterIds?: string[];
	}

	let addUserGroupDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let users = $state<OrgUser[]>([]);
	let isDialogOpen = $state(false);
	let loading = $state(false);
	let searchNames = $state('');
	let selectedUsers = $state<(OrgUser | OrgGroup)[]>([]);
	let selectedUsersMap = $derived(new Set(selectedUsers.map((user) => user.id)));

	// Separate filtered states
	let filteredUsers = $state<OrgUser[]>([]);
	let filteredGroups = $state<OrgGroup[]>([]);

	// Combined filtered data derived from the two states
	let filteredData = $derived.by(() => {
		const everyoneGroup: OrgGroup = { id: '*', name: 'Everyone' };
		const shouldIncludeEveryone =
			!searchNames.length || everyoneGroup.name.toLowerCase().includes(searchNames.toLowerCase());

		const allGroups = shouldIncludeEveryone ? [everyoneGroup, ...filteredGroups] : filteredGroups;
		const combined: (OrgUser | OrgGroup)[] = [...allGroups, ...filteredUsers];
		const filterIdSet = new Set(filterIds);

		return combined.filter((item) => !filterIdSet.has(item.id));
	});

	async function search() {
		filteredUsers =
			searchNames.length > 0
				? users.filter(
						(user) =>
							user.email.toLowerCase().includes(searchNames.toLowerCase()) ||
							user.username.toLowerCase().includes(searchNames.toLowerCase())
					)
				: users;

		filteredGroups = await AdminService.listGroups(
			searchNames.length > 0 ? { query: searchNames } : undefined
		);
	}

	export function open() {
		addUserGroupDialog?.open();
	}

	async function onOpen() {
		isDialogOpen = true;
		loading = true;

		// Load initial data
		try {
			filteredUsers = await AdminService.listUsers();
		} catch (error) {
			console.error('Error loading initial data:', error);
		} finally {
			loading = false;
		}
	}

	function onClose() {
		isDialogOpen = false;
		loading = false;
		searchNames = '';
		selectedUsers = [];
		users = [];
		filteredUsers = [];
		filteredGroups = [];
	}

	let { onAdd, filterIds }: Props = $props();

	$effect(() => {
		if (!isDialogOpen) {
			loading = false;
			return;
		}

		if (searchNames === '') {
			loading = false;
			users = [];
			filteredGroups = [];
			return;
		}

		loading = true;
		search().finally(() => {
			loading = false;
		});
	});
</script>

<ResponsiveDialog
	bind:this={addUserGroupDialog}
	{onClose}
	{onOpen}
	title="Add User/Group"
	class="h-full w-full overflow-visible p-0 md:h-[500px] md:max-w-md"
	classes={{ header: 'p-4 md:pb-0' }}
>
	<div class="default-scrollbar-thin flex grow flex-col gap-4 overflow-y-auto pt-1">
		<div class="px-4">
			<Search
				class="dark:bg-surface1 dark:border-surface3 shadow-inner dark:border"
				onChange={(val) => (searchNames = val)}
				placeholder="Search by user name, email, or group name..."
			/>
		</div>
		{#if loading}
			<div class="flex grow items-center justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:else}
			<div class="flex flex-col">
				{#each filteredData ?? [] as item (item.id)}
					<button
						class={twMerge(
							'dark:hover:bg-surface1 hover:bg-surface2 flex items-center gap-2 px-4 py-2 text-left',
							selectedUsersMap.has(item.id) && 'dark:bg-gray-920 bg-gray-50'
						)}
						onclick={() => {
							if (selectedUsersMap.has(item.id)) {
								const index = selectedUsers.findIndex((u) => u.id === item.id);
								if (index !== -1) {
									selectedUsers.splice(index, 1);
								}
							} else {
								selectedUsers.push(item);
								selectedUsersMap.add(item.id);
							}
						}}
					>
						{#if item.iconURL}
							<img
								src={item.iconURL}
								alt={'username' in item ? item.username : item.name}
								class="size-10 rounded-full"
							/>
						{:else}
							<Users class="size-10 rounded-full p-2" />
						{/if}
						<div class="flex grow flex-col">
							{#if 'email' in item}
								<p>{item.email}</p>
								<p class="font-light text-gray-400 dark:text-gray-600">
									{item.role === Role.ADMIN ? 'Admin' : 'User'}
								</p>
							{:else}
								<p>{item.name}</p>
								<p class="font-light text-gray-400 dark:text-gray-600">Group</p>
							{/if}
						</div>
						<div class="flex items-center justify-center">
							{#if selectedUsersMap.has(item.id)}
								<Check class="size-6 text-blue-500" />
							{/if}
						</div>
					</button>
				{/each}
			</div>
		{/if}
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
		<div class="flex items-center gap-2">
			<button class="button w-full md:w-fit" onclick={() => addUserGroupDialog?.close()}>
				Cancel
			</button>
			<button
				class="button-primary w-full md:w-fit"
				onclick={() => {
					const users = selectedUsers.filter((user) => 'email' in user) as OrgUser[];
					const groups = selectedUsers.filter((user) => !('email' in user)) as OrgGroup[];
					onAdd(users, groups);
					addUserGroupDialog?.close();
				}}
			>
				Confirm
			</button>
		</div>
	</div>
</ResponsiveDialog>
