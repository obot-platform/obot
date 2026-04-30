<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService } from '$lib/services';
	import { type OrgGroup, type OrgUser } from '$lib/services/admin/types';
	import { getUserRoleLabel } from '$lib/utils';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import Search from '../Search.svelte';
	import { debounce } from 'es-toolkit';
	import { Check, User, Users } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onAdd: (users: OrgUser[], groups: OrgGroup[]) => void;
		filterIds?: string[];
		initialUsers?: OrgUser[];
		initialGroups?: OrgGroup[];
	}

	let { onAdd, filterIds, initialUsers = [], initialGroups = [] }: Props = $props();

	let addUserGroupDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let users = $state<OrgUser[]>([]);
	let groups = $state<OrgGroup[]>([]);
	let loading = $state(false);
	let searchNames = $state('');
	let selectedUsers = $state<(OrgUser | OrgGroup)[]>([]);
	let selectedUsersMap = $derived(new Set(selectedUsers.map((user) => user.id)));
	let filteredUsers = $state<OrgUser[]>([]);
	let filteredGroups = $state<OrgGroup[]>([]);

	$effect(() => {
		if (initialUsers.length > 0) {
			users = initialUsers;
		}
		if (initialGroups.length > 0) {
			groups = initialGroups;
		}
	});

	function isGroup(item: OrgUser | OrgGroup): item is OrgGroup {
		return 'name' in item;
	}

	let filteredData = $derived.by(() => {
		const everyoneGroup: OrgGroup = { id: '*', name: 'All Obot Users' };
		const shouldIncludeEveryone =
			!searchNames.length || everyoneGroup.name.toLowerCase().includes(searchNames.toLowerCase());

		const allGroups = shouldIncludeEveryone ? [everyoneGroup, ...filteredGroups] : filteredGroups;
		const combined: (OrgUser | OrgGroup)[] = [...allGroups, ...filteredUsers];
		const filterIdSet = new Set(filterIds ?? []);

		return combined.filter((item) => !filterIdSet.has(item.id));
	});

	async function search() {
		loading = true;

		filteredUsers =
			searchNames.length > 0
				? users.filter(
						(user) =>
							(user.displayName ?? '').toLowerCase().includes(searchNames.toLowerCase()) ||
							(user.email ?? '').toLowerCase().includes(searchNames.toLowerCase()) ||
							(user.username ?? '').toLowerCase().includes(searchNames.toLowerCase())
					)
				: users;

		try {
			// Fetch groups with server-side search
			filteredGroups =
				searchNames.length === 0 && groups.length > 0
					? [...groups].sort((a, b) => a.name.localeCompare(b.name))
					: (
							await AdminService.listGroups(
								searchNames.length > 0 ? { query: searchNames } : undefined
							)
						).sort((a, b) => a.name.localeCompare(b.name));
			if (searchNames.length === 0) {
				groups = filteredGroups;
			}
		} catch (error) {
			console.error('Error loading groups:', error);
		} finally {
			loading = false;
		}
	}

	const handleSearch = debounce(() => {
		// Debounce search to avoid making too many requests
		search();
	}, 500);

	export function open() {
		searchNames = '';
		addUserGroupDialog?.open();
	}

	async function onOpen() {
		loading = true;

		try {
			if (users.length === 0) {
				users = await AdminService.listUsers();
			}
		} catch (error) {
			console.error('Error loading initial users:', error);
		} finally {
			loading = false;
		}

		// Now search to populate filtered data
		await search();
	}

	function onClose() {
		loading = false;
		searchNames = '';
		selectedUsers = [];
		filteredUsers = [];
		filteredGroups = [];
	}
</script>

<ResponsiveDialog
	bind:this={addUserGroupDialog}
	{onClose}
	{onOpen}
	title="Add User/Group"
	class="h-full w-full overflow-visible md:h-[500px] md:max-w-md"
	classes={{ header: 'p-4 md:pb-0', content: 'min-h-inherit p-0' }}
>
	<div class="default-scrollbar-thin flex grow flex-col gap-4 overflow-y-auto pt-1">
		<div class="px-4">
			<Search
				class="dark:bg-base-200 dark:border-base-400 shadow-inner dark:border"
				value={searchNames}
				onChange={(val) => {
					searchNames = val;
					handleSearch();
				}}
				placeholder="Search by user name, email, or group name..."
			/>
		</div>
		{#if loading}
			<div class="flex grow items-center justify-center">
				<Loading class="size-6" />
			</div>
		{:else}
			<div class="flex flex-col">
				{#each filteredData ?? [] as item (item.id)}
					<button
						class={twMerge(
							'dark:hover:bg-base-200 hover:bg-base-400 flex items-center gap-2 px-4 py-2 text-left',
							selectedUsersMap.has(item.id) && 'bg-base-200/50'
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
						<div class="flex grow flex-col">
							{#if !isGroup(item)}
								<p>{item.displayName ?? item.email ?? item.username ?? item.id}</p>
								<p class="text-muted-content font-light">
									{item.effectiveRole ? getUserRoleLabel(item.effectiveRole) : 'User'}
								</p>
							{:else}
								<p>{item.name}</p>
								<p class="text-muted-content font-light">Group</p>
							{/if}
						</div>
						<div class="flex items-center justify-center">
							{#if selectedUsersMap.has(item.id)}
								<Check class="text-primary size-6" />
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
			<button class="btn btn-secondary w-full md:w-fit" onclick={() => addUserGroupDialog?.close()}>
				Cancel
			</button>
			<button
				class="btn btn-primary w-full md:w-fit"
				onclick={() => {
					const users = selectedUsers.filter((user) => !isGroup(user)) as OrgUser[];
					const groups = selectedUsers.filter((user) => isGroup(user)) as OrgGroup[];
					onAdd(users, groups);
					addUserGroupDialog?.close();
				}}
			>
				Confirm
			</button>
		</div>
	</div>
</ResponsiveDialog>
