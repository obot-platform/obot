<script lang="ts">
	import { MCP_ACCESS_POLICY_FIELD_IDS } from '$lib/constants';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type OrgGroup, type OrgUser } from '$lib/services';
	import AgentIdentitiesService, { type AgentIdentity } from '$lib/services/agent-identities';
	import { profile } from '$lib/stores';
	import {
		pickerWildcardKind,
		subjectPickerOptions,
		wildcardConflicts,
		type SubjectPickerOption
	} from '$lib/subjectResolver';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import Search from '../Search.svelte';
	import { Check, TriangleAlert, User, Users } from '@lucide/svelte';
	import { debounce } from 'es-toolkit';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		// The wildcards arrive as groups: "*" for everyone, "users:*" and "agents:*" when agents are offered.
		onAdd: (users: OrgUser[], groups: OrgGroup[], agents: AgentIdentity[]) => void;
		// filterIds are the picker IDs of the subjects already added (see resolveSubjectPickerById). They are
		// not offered again, and they decide which wildcards still can be.
		filterIds?: string[];
		initialUsers?: OrgUser[];
		// offerObotAdmin offers the Obot Admin group, which only subjects that accept Obot groups can hold.
		offerObotAdmin?: boolean;
		// offerAgents offers agents, and the wildcards "All Users" and "All Agents" next to "Everyone", which
		// includes agents. Only subjects that accept agents can use it.
		offerAgents?: boolean;
		// wildcardMustBeAlone is for subjects where a wildcard has to be the only subject.
		wildcardMustBeAlone?: boolean;
	}

	let {
		onAdd,
		filterIds,
		initialUsers = [],
		offerObotAdmin = true,
		offerAgents = false,
		wildcardMustBeAlone = false
	}: Props = $props();

	let addUserGroupDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let users = $state<OrgUser[]>([]);
	let loading = $state(false);
	let searchNames = $state('');
	let agents = $state<AgentIdentity[]>([]);
	let selectedUsers = $state<SubjectPickerOption[]>([]);
	let selectedUsersMap = $derived(new Set(selectedUsers.map((item) => item.key)));
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

	let filteredData = $derived(
		subjectPickerOptions({
			users,
			groups: filteredGroups,
			agents,
			query: searchNames,
			offerAgents,
			offerObotAdmin: offerObotAdmin && Boolean(profile.current.isAdmin?.()),
			existingPickerIds: filterIds ?? []
		})
	);

	// The wildcards that cannot be added next to what is already added or selected: everyone is not
	// combined with the other wildcards, and with wildcardMustBeAlone a wildcard is the only subject.
	function isBlocked(item: SubjectPickerOption): boolean {
		if (selectedUsersMap.has(item.key)) return false;
		const others = [...(filterIds ?? []), ...selectedUsers.map((selected) => selected.key)];
		if (pickerWildcardKind(item.key)) {
			if (wildcardConflicts(item.key, others)) return true;
			return wildcardMustBeAlone && others.length > 0;
		}
		return wildcardMustBeAlone && others.some((other) => pickerWildcardKind(other));
	}

	async function search() {
		if (!dialogOpen) {
			return;
		}

		inFlight?.abort();
		const controller = new AbortController();
		inFlight = controller;

		loading = true;

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
		let loadedAgents: AgentIdentity[] | undefined;
		try {
			[loaded, loadedAgents] = await Promise.all([
				users.length === 0 ? UserService.listUsers() : undefined,
				offerAgents
					? AgentIdentitiesService.listAgents().catch((error) => {
							console.error('Error loading agents:', error);
							return undefined;
						})
					: undefined
			]);
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
		if (loadedAgents) {
			agents = loadedAgents;
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
	title={offerAgents ? m.core_add_user_group_agent() : m.core_add_user_group()}
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
				placeholder={offerAgents
					? m.core_search_users_groups_agents()
					: m.identity_access_users_search_users_groups()}
			/>
		</div>
		{#if groupsDegraded}
			<div
				class="mx-4 flex items-start gap-2 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-xs"
				role="status"
			>
				<TriangleAlert class="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400" />
				<span>
					{m.identity_access_users_groups_degraded_notice()}
				</span>
			</div>
		{:else if groupsHasMore}
			<p class="text-muted-content px-4 text-xs">
				{m.identity_access_users_groups_has_more({ count: filteredGroups.length })}
			</p>
		{/if}
		{#if loading}
			<div class="flex grow items-center justify-center">
				<Loading class="size-6" />
			</div>
		{:else}
			<div class="flex flex-col">
				{#each filteredData ?? [] as item (item.key)}
					{@const blocked = isBlocked(item)}
					<button
						id={item.key === '*' ? MCP_ACCESS_POLICY_FIELD_IDS.allUsersOption : undefined}
						class={twMerge(
							'dark:hover:bg-base-200 hover:bg-base-400 flex items-center gap-2 px-4 py-2 text-left',
							selectedUsersMap.has(item.key) && 'bg-base-200/50',
							blocked && 'cursor-not-allowed opacity-50 hover:bg-transparent'
						)}
						disabled={blocked}
						title={blocked ? m.core_wildcard_unavailable() : undefined}
						onclick={() => {
							if (selectedUsersMap.has(item.key)) {
								const index = selectedUsers.findIndex((u) => u.key === item.key);
								if (index !== -1) {
									selectedUsers.splice(index, 1);
								}
							} else {
								selectedUsers.push(item);
							}
						}}
					>
						<div class="flex grow flex-col">
							<p>{item.label}</p>
							<p class="text-muted-content font-light">{item.kind}</p>
						</div>
						<div class="flex items-center justify-center">
							{#if selectedUsersMap.has(item.key)}
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
				{m.core_n_selected({ count: selectedUsers.length })}
			{/if}
		</div>
		<div class="flex items-center gap-2 md:flex-row flex-col">
			<button class="btn btn-secondary w-full md:w-fit" onclick={() => addUserGroupDialog?.close()}>
				{m.common_cancel()}
			</button>
			<button
				id={MCP_ACCESS_POLICY_FIELD_IDS.userGroupConfirmBtn}
				class="btn btn-primary w-full md:w-fit"
				onclick={() => {
					onAdd(
						selectedUsers.flatMap((item) => (item.user ? [item.user] : [])),
						selectedUsers.flatMap((item) => (item.group ? [item.group] : [])),
						selectedUsers.flatMap((item) => (item.agent ? [item.agent] : []))
					);
					addUserGroupDialog?.close();
				}}
			>
				{m.core_confirm()}
			</button>
		</div>
	</div>
</ResponsiveDialog>
