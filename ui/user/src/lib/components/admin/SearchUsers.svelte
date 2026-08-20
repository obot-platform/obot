<script lang="ts">
	import { MCP_ACCESS_POLICY_FIELD_IDS } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type OrgGroup, type OrgUser } from '$lib/services';
	import { getUserRoleLabel } from '$lib/utils';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import Search from '../Search.svelte';
	import { Check, TriangleAlert, User, Users } from '@lucide/svelte';
	import { debounce } from 'es-toolkit';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onAdd: (users: OrgUser[], groups: OrgGroup[]) => void;
		filterIds?: string[];
		initialUsers?: OrgUser[];
	}

	let { onAdd, filterIds, initialUsers = [] }: Props = $props();

	let addUserGroupDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let users = $state<OrgUser[]>([]);
	let loading = $state(false);
	let searchNames = $state('');
	let selectedUsers = $state<(OrgUser | OrgGroup)[]>([]);
	let selectedUsersMap = $derived(new Set(selectedUsers.map((user) => user.id)));
	let filteredUsers = $state<OrgUser[]>([]);
	let filteredGroups = $state<OrgGroup[]>([]);
	let groupsHasMore = $state(false);
	let groupsDegraded = $state(false);

	// Only the first page of groups is fetched; the search box narrows it server-side.
	const GROUP_PAGE_SIZE = 50;

	// Searches are debounced but not serialized, so a slow request for an earlier query can land
	// after a fast one for a later query. Only the newest request is allowed to write state.
	let inFlight: AbortController | undefined;

	let dialogOpen = false;

	$effect(() => {
		if (initialUsers.length > 0) {
			users = initialUsers;
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
		if (!dialogOpen) {
			return;
		}

		inFlight?.abort();
		const controller = new AbortController();
		inFlight = controller;

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
			// Groups are searched and paged server-side: a directory can hold far more than can be
			// listed at once, so only the first page is fetched and the user narrows with the query.
			const page = await UserService.listGroups({
				query: searchNames.length > 0 ? searchNames : undefined,
				limit: GROUP_PAGE_SIZE,
				signal: controller.signal
			});
			if (controller.signal.aborted || !dialogOpen) return;

			filteredGroups = [...page.items].sort((a, b) => a.name.localeCompare(b.name));
			groupsHasMore = Boolean(page.nextCursor);
			groupsDegraded = page.degraded;
		} catch (error) {
			if (controller.signal.aborted) return;
			console.error('Error loading groups:', error);
		} finally {
			if (!controller.signal.aborted) {
				loading = false;
			}
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
		dialogOpen = true;
		loading = true;

		let loaded: OrgUser[] | undefined;
		try {
			if (users.length === 0) {
				loaded = await UserService.listUsers();
			}
		} catch (error) {
			console.error('Error loading initial users:', error);
		} finally {
			loading = false;
		}

		// The dialog can be closed while the user list is still loading.
		if (!dialogOpen) {
			return;
		}
		if (loaded) {
			users = loaded;
		}

		// Now search to populate filtered data
		await search();
	}

	function onClose() {
		dialogOpen = false;
		handleSearch.cancel();
		inFlight?.abort();
		inFlight = undefined;

		loading = false;
		searchNames = '';
		selectedUsers = [];
		filteredUsers = [];
		filteredGroups = [];
		groupsHasMore = false;
		groupsDegraded = false;
	}
</script>

<ResponsiveDialog
	id="add-user-group-dialog"
	bind:this={addUserGroupDialog}
	{onClose}
	{onOpen}
	title="Add User/Group"
	class="h-full w-full overflow-visible md:h-125 md:max-w-md"
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
		{#if groupsDegraded}
			<div
				class="mx-4 flex items-start gap-2 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-xs"
				role="status"
			>
				<TriangleAlert class="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400" />
				<span>
					Showing only groups Obot has already recorded &mdash; directory-wide group read permission
					may not be granted.
				</span>
			</div>
		{:else if groupsHasMore}
			<p class="text-muted-content px-4 text-xs">
				Showing the first {filteredGroups.length} groups. Refine your search to narrow the list.
			</p>
		{/if}
		{#if loading}
			<div class="flex grow items-center justify-center">
				<Loading class="size-6" />
			</div>
		{:else}
			<div class="flex flex-col">
				{#each filteredData ?? [] as item (item.id)}
					<button
						id={item.id === '*' ? MCP_ACCESS_POLICY_FIELD_IDS.allUsersOption : undefined}
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
				id={MCP_ACCESS_POLICY_FIELD_IDS.userGroupConfirmBtn}
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
