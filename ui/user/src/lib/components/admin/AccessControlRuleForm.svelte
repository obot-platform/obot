<script lang="ts">
	import {
		PAGE_TRANSITION_DURATION,
		ADMIN_SESSION_STORAGE,
		DEFAULT_MCP_CATALOG_ID,
		ADMIN_ALL_OPTION
	} from '$lib/constants';
	import { AdminService, ChatService } from '$lib/services';
	import {
		type AccessControlRule,
		type AccessControlRuleManifest,
		type AccessControlRuleResource,
		type AccessControlRuleSubject,
		type OrgUser,
		type OrgGroup
	} from '$lib/services/admin/types';
	import { LoaderCircle, Plus, Trash2 } from 'lucide-svelte';
	import { type Snippet } from 'svelte';
	import { fly } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Table from '../table/Table.svelte';
	import SearchUsers from './SearchUsers.svelte';
	import Confirm from '../Confirm.svelte';
	import { goto } from '$app/navigation';
	import SearchMcpServers from './SearchMcpServers.svelte';
	import type { AdminMcpServerAndEntriesContext } from '$lib/context/admin/mcpServerAndEntries.svelte';
	import type { PoweruserWorkspaceContext } from '$lib/context/poweruserWorkspace.svelte';
	import { getRegistryLabel, getUserDisplayName } from '$lib/utils';
	import { profile } from '$lib/stores';

	interface Props {
		topContent?: Snippet;
		accessControlRule?: AccessControlRule;
		onCreate?: (accessControlRule: AccessControlRule) => void;
		onUpdate?: (accessControlRule: AccessControlRule) => void;
		entity?: 'workspace' | 'catalog';
		id?: string | null;
		mcpEntriesContextFn: () => AdminMcpServerAndEntriesContext | PoweruserWorkspaceContext;
		all?: { label: string; description: string };
		readonly?: boolean;
		isAdminView?: boolean;
	}

	let {
		topContent,
		accessControlRule: initialAccessControlRule,
		onCreate,
		onUpdate,
		mcpEntriesContextFn,
		readonly,
		isAdminView,
		all = ADMIN_ALL_OPTION,
		id = DEFAULT_MCP_CATALOG_ID,
		entity = 'catalog'
	}: Props = $props();
	const duration = PAGE_TRANSITION_DURATION;
	let accessControlRule = $state(
		initialAccessControlRule ??
			({
				displayName: '',
				userIDs: [],
				mcpServerCatalogEntryNames: [],
				mcpServerNames: []
			} as AccessControlRuleManifest)
	);

	let saving = $state<boolean | undefined>();
	let redirect = $state('');
	let usersAndGroups = $state<{ users: OrgUser[]; groups: OrgGroup[] }>();
	let loadingUsersAndGroups = $state(false);

	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();
	let addMcpServerDialog = $state<ReturnType<typeof SearchMcpServers>>();

	let deletingRule = $state(false);

	const mcpServerAndEntries = mcpEntriesContextFn?.() ?? {
		entries: [],
		servers: [],
		loading: false
	};
	let usersMap = $derived(new Map(usersAndGroups?.users.map((user) => [user.id, user]) ?? []));
	let mcpServersMap = $derived(new Map(mcpServerAndEntries.servers.map((i) => [i.id, i])));
	let mcpEntriesMap = $derived(new Map(mcpServerAndEntries.entries.map((i) => [i.id, i])));
	let mcpServersTableData = $derived.by(() => {
		if (mcpServersMap && mcpEntriesMap) {
			return convertMcpServersToTableData(accessControlRule.resources ?? []);
		}
		return [];
	});

	$effect(() => {
		// Prevent loading users and groups if acr has no subjects
		if (!accessControlRule.subjects || accessControlRule.subjects?.length === 0) {
			return;
		}

		loadingUsersAndGroups = true;

		// Prevent refetching when adding new users or groups
		const promises: [Promise<OrgUser[] | undefined>, Promise<OrgGroup[] | undefined>] = [
			Promise.resolve(undefined),
			Promise.resolve(undefined)
		];

		if (!usersAndGroups?.users) {
			promises[0] = AdminService.listUsers();
		}
		if (!usersAndGroups?.groups) {
			// Include restricted groups in the results so that groups added to ACRs before the group
			// restriction was configured are still visible in the UI.
			promises[1] = AdminService.listGroups({ includeRestricted: true });
		}

		Promise.all(promises)
			.then(([users, groups]) => {
				if (!usersAndGroups) {
					usersAndGroups = { users: [], groups: [] };
				}

				if (users) {
					usersAndGroups!.users = users;
				}

				if (groups) {
					usersAndGroups!.groups = groups;
				}

				loadingUsersAndGroups = false;
			})
			.catch((error) => {
				console.error('Failed to load users and groups:', error);
				loadingUsersAndGroups = false;
			});
	});

	$effect(() => {
		const initialAdditionId = sessionStorage.getItem(
			ADMIN_SESSION_STORAGE.ACCESS_CONTROL_RULE_CREATION
		);
		if (
			initialAdditionId &&
			!mcpServerAndEntries.loading &&
			(mcpServersMap.size > 0 || mcpEntriesMap.size > 0)
		) {
			// Check if this resource is already added to prevent duplicates
			const existingResourceIds = new Set(
				accessControlRule.resources?.map((resource) => resource.id) ?? []
			);

			if (!existingResourceIds.has(initialAdditionId)) {
				const entry = mcpEntriesMap.get(initialAdditionId);
				if (entry) {
					accessControlRule.resources = [
						...(accessControlRule.resources ?? []),
						{ id: entry.id, type: 'mcpServerCatalogEntry' }
					];
					redirect =
						entity === 'workspace'
							? `/mcp-servers/c/${entry.id}`
							: `/admin/mcp-servers/c/${entry.id}`;
				} else {
					const server = mcpServersMap.get(initialAdditionId);
					if (server) {
						accessControlRule.resources = [
							...(accessControlRule.resources ?? []),
							{ id: server.id, type: 'mcpServer' }
						];
						redirect =
							entity === 'workspace'
								? `/mcp-servers/s/${server.id}`
								: `/admin/mcp-servers/s/${server.id}`;
					}
				}
			}
			sessionStorage.removeItem(ADMIN_SESSION_STORAGE.ACCESS_CONTROL_RULE_CREATION);
		}
	});

	function convertSubjectsToTableData(
		subjects: AccessControlRuleSubject[],
		users: OrgUser[],
		groups: OrgGroup[]
	) {
		const userMap = new Map(users?.map((user) => [user.id, user]));
		const groupMap = new Map(groups?.map((group) => [group.id, group]));

		return (
			subjects
				.map((subject) => {
					if (subject.type === 'user') {
						return {
							id: subject.id,
							displayName: getUserDisplayName(userMap, subject.id),
							type: 'User'
						};
					}

					if (subject.type === 'group') {
						const group = groupMap.get(subject.id);
						if (!group) {
							return undefined;
						}

						return {
							id: subject.id,
							displayName: group.name,
							type: 'Group'
						};
					}

					return {
						id: subject.id,
						displayName: subject.id === '*' ? 'All Obot Users' : subject.id,
						type: 'Group'
					};
				})
				.filter((subject) => subject !== undefined) ?? []
		);
	}

	function convertMcpServersToTableData(resources: AccessControlRuleResource[]) {
		const owner = initialAccessControlRule?.powerUserID
			? getUserDisplayName(usersMap, initialAccessControlRule.powerUserID)
			: undefined;
		const isMe = initialAccessControlRule?.powerUserID === profile.current?.id;
		return resources.map((resource) => {
			if (resource.type === 'mcpServerCatalogEntry') {
				const entry = mcpEntriesMap.get(resource.id);
				return {
					id: resource.id,
					name: entry?.manifest?.name || '-',
					type: 'mcpentry'
				};
			}

			if (resource.type === 'mcpServer') {
				const server = mcpServersMap.get(resource.id);
				return {
					id: resource.id,
					name: server?.manifest.name || '-',
					type: 'mcpserver'
				};
			}

			const allLabel = owner
				? isMe
					? 'Everything In My Registry'
					: `Everything In ${owner}'s Registry`
				: all.label;

			return {
				id: resource.id,
				name: resource.id === '*' ? allLabel : resource.id,
				type: 'selector'
			};
		});
	}

	function validate(rule: typeof accessControlRule) {
		if (!rule) return false;

		return rule.displayName.length > 0;
	}
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div class="flex grow flex-col gap-4" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		{#if topContent}
			{@render topContent()}
		{/if}
		{#if accessControlRule.id}
			<div class="flex w-full items-center justify-between gap-4">
				<div class="flex items-center gap-2">
					<h1 class="flex items-center gap-4 text-2xl font-semibold">
						{accessControlRule.displayName}
					</h1>
					{#if !loadingUsersAndGroups}
						{#if initialAccessControlRule}
							{@const registry = getRegistryLabel(
								initialAccessControlRule.powerUserID,
								profile.current?.id,
								usersAndGroups?.users
							)}
							{#if registry}
								<div class="dark:bg-surface2 bg-surface3 rounded-full px-3 py-1 text-xs">
									{registry}
								</div>
							{/if}
						{/if}
					{/if}
				</div>
				{#if !readonly}
					<button
						class="button-destructive flex items-center gap-1 text-xs font-normal"
						use:tooltip={'Delete Catalog'}
						onclick={() => {
							deletingRule = true;
						}}
					>
						<Trash2 class="size-4" />
					</button>
				{/if}
			</div>
		{/if}

		{#if !accessControlRule.id}
			<div
				class="dark:bg-surface2 dark:border-surface3 bg-background rounded-lg border border-transparent p-4"
			>
				<div class="flex flex-col gap-6">
					<div class="flex flex-col gap-2">
						<label for="mcp-catalog-name" class="flex-1 text-sm font-light capitalize">
							Name
						</label>
						<input
							id="mcp-catalog-name"
							bind:value={accessControlRule.displayName}
							class="text-input-filled mt-0.5"
							disabled={readonly}
						/>
					</div>
				</div>
			</div>
		{/if}

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">User & Groups</h2>
				{#if !readonly}
					<div class="relative flex items-center gap-4">
						{#if loadingUsersAndGroups}
							<button class="button-primary flex items-center gap-1 text-sm" disabled>
								<Plus class="size-4" /> Add User/Group
							</button>
						{:else}
							<button
								class="button-primary flex items-center gap-1 text-sm"
								onclick={() => {
									addUserGroupDialog?.open();
								}}
							>
								<Plus class="size-4" /> Add User/Group
							</button>
						{/if}
					</div>
				{/if}
			</div>
			{#if loadingUsersAndGroups}
				<div class="my-2 flex items-center justify-center">
					<LoaderCircle class="size-6 animate-spin" />
				</div>
			{:else}
				{@const tableData = convertSubjectsToTableData(
					accessControlRule.subjects ?? [],
					usersAndGroups?.users ?? [],
					usersAndGroups?.groups ?? []
				)}
				<Table
					data={tableData}
					fields={['displayName', 'type']}
					headers={[{ property: 'displayName', title: 'Name' }]}
					noDataMessage="No users or groups added."
				>
					{#snippet actions(d)}
						{#if !readonly}
							<button
								class="icon-button hover:text-red-500"
								onclick={() => {
									accessControlRule.subjects = accessControlRule.subjects?.filter(
										(subject) => subject.id !== d.id
									);
								}}
								use:tooltip={'Delete User/Group'}
							>
								<Trash2 class="size-4" />
							</button>
						{/if}
					{/snippet}
				</Table>
			{/if}
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">MCP Servers</h2>
				{#if !readonly}
					<div class="relative flex items-center gap-4">
						<button
							class="button-primary flex items-center gap-1 text-sm"
							onclick={() => {
								addMcpServerDialog?.open();
							}}
						>
							<Plus class="size-4" /> Add MCP Server
						</button>
					</div>
				{/if}
			</div>
			<Table data={mcpServersTableData} fields={['name']} noDataMessage="No MCP servers added.">
				{#snippet actions(d)}
					{#if !readonly}
						<button
							class="icon-button hover:text-red-500"
							onclick={() => {
								accessControlRule.resources =
									accessControlRule.resources?.filter((resource) => resource.id !== d.id) ?? [];
							}}
							use:tooltip={'Remove MCP Server'}
						>
							<Trash2 class="size-4" />
						</button>
					{/if}
				{/snippet}
			</Table>
		</div>
	</div>
	{#if !readonly}
		<div
			class="bg-surface1 text-on-surface1 dark:bg-background sticky bottom-0 left-0 flex w-full justify-end gap-2 py-4"
			out:fly={{ x: -100, duration }}
			in:fly={{ x: -100 }}
		>
			<div class="flex w-full justify-end gap-2">
				{#if !accessControlRule.id}
					<button
						class="button text-sm"
						onclick={() => {
							if (redirect) {
								goto(redirect);
							} else {
								goto('/admin/access-control');
							}
						}}
					>
						Cancel
					</button>
					<button
						class="button-primary text-sm"
						disabled={!validate(accessControlRule) || saving}
						onclick={async () => {
							if (!id) return;
							saving = true;
							const response =
								entity === 'workspace'
									? await ChatService.createWorkspaceAccessControlRule(id, accessControlRule)
									: await AdminService.createAccessControlRule(accessControlRule);
							accessControlRule = response;
							if (redirect) {
								goto(redirect);
							} else {
								onCreate?.(response);
							}
							saving = false;
						}}
					>
						{#if saving}
							<LoaderCircle class="size-4 animate-spin" />
						{:else}
							Save
						{/if}
					</button>
				{:else}
					<button
						class="button text-sm"
						disabled={saving}
						onclick={async () => {
							if (!accessControlRule.id || !id) return;
							saving = true;
							accessControlRule =
								entity === 'workspace'
									? await ChatService.getWorkspaceAccessControlRule(id, accessControlRule.id)
									: await AdminService.getAccessControlRule(accessControlRule.id);
							saving = false;
						}}
					>
						Reset
					</button>
					<button
						class="button-primary text-sm"
						disabled={!validate(accessControlRule) || saving}
						onclick={async () => {
							if (!accessControlRule.id || !id) return;
							saving = true;
							const response =
								entity === 'workspace'
									? await ChatService.updateWorkspaceAccessControlRule(
											id,
											accessControlRule.id,
											accessControlRule
										)
									: await AdminService.updateAccessControlRule(
											accessControlRule.id,
											accessControlRule
										);
							accessControlRule = response;
							onUpdate?.(response);
							saving = false;
						}}
					>
						{#if saving}
							<LoaderCircle class="size-4 animate-spin" />
						{:else}
							Update
						{/if}
					</button>
				{/if}
			</div>
		</div>
	{/if}
</div>

<SearchUsers
	bind:this={addUserGroupDialog}
	filterIds={accessControlRule.subjects?.map((subject) => subject.id) ?? []}
	onAdd={async (users: OrgUser[], groups: OrgGroup[]) => {
		const existingSubjectIds = new Set(
			accessControlRule.subjects?.map((subject) => subject.id) ?? []
		);
		const newSubjects = [
			...users
				.filter((user: OrgUser) => !existingSubjectIds.has(user.id))
				.map((user: OrgUser) => ({
					type: 'user' as const,
					id: user.id
				})),
			...groups
				.filter((group: OrgGroup) => !existingSubjectIds.has(group.id))
				.map((group: OrgGroup) => ({
					type: group.id === '*' ? ('selector' as const) : ('group' as const),
					id: group.id
				}))
		];
		accessControlRule.subjects = [...(accessControlRule.subjects ?? []), ...newSubjects];
	}}
/>

<SearchMcpServers
	bind:this={addMcpServerDialog}
	type="acr"
	exclude={accessControlRule.resources?.map((resource) => resource.id) ?? []}
	onAdd={async (mcpCatalogEntryIds, mcpServerIds, otherSelectors) => {
		const existingResourceIds = new Set(
			accessControlRule.resources?.map((resource) => resource.id) ?? []
		);
		const newEntryResources = mcpCatalogEntryIds.filter((id) => !existingResourceIds.has(id));
		const newServerResources = mcpServerIds.filter((id) => !existingResourceIds.has(id));

		accessControlRule.resources = [
			...(accessControlRule.resources ?? []),
			...newEntryResources.map((id) => ({ type: 'mcpServerCatalogEntry' as const, id })),
			...newServerResources.map((id) => ({ type: 'mcpServer' as const, id })),
			...otherSelectors.map((id) => ({ type: 'selector' as const, id }))
		];
	}}
	{mcpEntriesContextFn}
	{all}
	{entity}
	{isAdminView}
	workspaceId={entity === 'workspace' ? id : undefined}
/>

<Confirm
	msg="Are you sure you want to delete this rule?"
	show={deletingRule}
	onsuccess={async () => {
		if (!accessControlRule.id || !id) return;
		saving = true;
		await (entity === 'workspace'
			? ChatService.deleteWorkspaceAccessControlRule(id, accessControlRule.id)
			: AdminService.deleteAccessControlRule(accessControlRule.id));
		goto('/admin/access-control');
	}}
	oncancel={() => (deletingRule = false)}
/>
