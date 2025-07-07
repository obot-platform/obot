<script lang="ts">
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService } from '$lib/services';
	import {
		Role,
		type AccessControlRule,
		type AccessControlRuleManifest,
		type AccessControlRuleResource,
		type AccessControlRuleSubject,
		type OrgUser
	} from '$lib/services/admin/types';
	import { LoaderCircle, Plus, Trash2 } from 'lucide-svelte';
	import { onMount, type Snippet } from 'svelte';
	import { fly } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Table from '../Table.svelte';
	import SearchUsers from './SearchUsers.svelte';
	import Confirm from '../Confirm.svelte';
	import { goto } from '$app/navigation';
	import SearchMcpServers from './SearchMcpServers.svelte';
	import { getAdminMcpServerAndEntries } from '$lib/context/admin/mcpServerAndEntries.svelte';

	interface Props {
		topContent?: Snippet;
		accessControlRule?: AccessControlRule;
		onCreate?: (accessControlRule: AccessControlRule) => void;
		onUpdate?: (accessControlRule: AccessControlRule) => void;
	}

	let {
		topContent,
		accessControlRule: initialAccessControlRule,
		onCreate,
		onUpdate
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
	let loadingUsers = $state<Promise<OrgUser[]>>();

	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();
	let addMcpServerDialog = $state<ReturnType<typeof SearchMcpServers>>();

	let deletingRule = $state(false);

	const adminMcpServerAndEntries = getAdminMcpServerAndEntries();
	let mcpServersMap = $derived(new Map(adminMcpServerAndEntries.servers.map((i) => [i.id, i])));
	let mcpEntriesMap = $derived(new Map(adminMcpServerAndEntries.entries.map((i) => [i.id, i])));
	let mcpServersTableData = $derived.by(() => {
		if (mcpServersMap && mcpEntriesMap) {
			return convertMcpServersToTableData(accessControlRule.resources ?? []);
		}
		return [];
	});

	onMount(async () => {
		loadingUsers = AdminService.listUsers();
	});

	function convertSubjectsToTableData(subjects: AccessControlRuleSubject[], users: OrgUser[]) {
		const userMap = new Map(users?.map((user) => [user.id, user]));
		return (
			subjects
				.map((subject) => {
					if (subject.type === 'user') {
						const user = userMap.get(subject.id);
						if (!user) {
							return undefined;
						}

						return {
							id: subject.id,
							displayName: user.email ?? user.username,
							role: user.role === Role.ADMIN ? 'Admin' : 'User',
							type: 'User'
						};
					}

					return {
						id: subject.id,
						displayName: subject.id === '*' ? 'Everyone' : subject.id,
						role: 'User',
						type: 'Group'
					};
				})
				.filter((user) => user !== undefined) ?? []
		);
	}

	function convertMcpServersToTableData(resources: AccessControlRuleResource[]) {
		return resources.map((resource) => {
			if (resource.type === 'mcpServerCatalogEntry') {
				const entry = mcpEntriesMap.get(resource.id);
				return {
					id: resource.id,
					name: entry?.commandManifest?.name || entry?.urlManifest?.name || '-',
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

			return {
				id: resource.id,
				name: resource.id === '*' ? 'Everything' : resource.id,
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
	class="flex h-full w-full flex-col gap-8"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div class="flex grow flex-col gap-8" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		{#if topContent}
			{@render topContent()}
		{/if}
		{#if accessControlRule.id}
			<div class="flex w-full items-center justify-between gap-4">
				<h1 class="flex items-center gap-4 text-2xl font-semibold">
					{accessControlRule.displayName}
				</h1>
				<button
					class="button-destructive flex items-center gap-1 text-xs font-normal"
					use:tooltip={'Delete Catalog'}
					onclick={() => {
						deletingRule = true;
					}}
				>
					<Trash2 class="size-4" />
				</button>
			</div>
		{:else}
			<h1 class="text-2xl font-semibold">Create Access Control Rule</h1>
		{/if}

		{#if !accessControlRule.id}
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
							bind:value={accessControlRule.displayName}
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
				{@const tableData = convertSubjectsToTableData(
					accessControlRule.subjects ?? [],
					users ?? []
				)}
				<Table
					data={tableData}
					fields={['displayName', 'type', 'role']}
					headers={[{ property: 'displayName', title: 'Name' }]}
					noDataMessage={'No users or groups added.'}
				>
					{#snippet actions(d)}
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
							addMcpServerDialog?.open();
						}}
					>
						<Plus class="size-4" /> Add MCP Server
					</button>
				</div>
			</div>
			<Table data={mcpServersTableData} fields={['name']} noDataMessage={'No MCP servers added.'}>
				{#snippet actions(d)}
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
				{/snippet}
			</Table>
		</div>
	</div>
	<div
		class="bg-surface1 sticky bottom-0 left-0 flex w-full justify-end gap-2 py-4 text-gray-400 dark:bg-black dark:text-gray-600"
		out:fly={{ x: -100, duration }}
		in:fly={{ x: -100 }}
	>
		<div class="flex w-full justify-end gap-2">
			{#if !accessControlRule.id}
				<button
					class="button text-sm"
					onclick={() => {
						goto('/v2/admin/access-control');
					}}
				>
					Cancel
				</button>
				<button
					class="button-primary text-sm disabled:opacity-75"
					disabled={!validate(accessControlRule) || saving}
					onclick={async () => {
						saving = true;
						const response = await AdminService.createAccessControlRule(accessControlRule);
						accessControlRule = response;
						onCreate?.(response);
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
						if (!accessControlRule.id) return;
						saving = true;
						accessControlRule = await AdminService.getAccessControlRule(accessControlRule.id);
						saving = false;
					}}
				>
					Reset
				</button>
				<button
					class="button-primary text-sm disabled:opacity-75"
					disabled={!validate(accessControlRule) || saving}
					onclick={async () => {
						if (!accessControlRule.id) return;
						saving = true;
						const response = await AdminService.updateAccessControlRule(
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
</div>

<SearchUsers
	bind:this={addUserGroupDialog}
	filterIds={accessControlRule.subjects?.map((subject) => subject.id) ?? []}
	onAdd={async (users, groups) => {
		const existingSubjectIds = new Set(
			accessControlRule.subjects?.map((subject) => subject.id) ?? []
		);
		const newSubjects = [
			...users
				.filter((user) => !existingSubjectIds.has(user.id))
				.map((user) => ({
					type: 'user' as const,
					id: user.id
				})),
			...groups
				.filter((group) => !existingSubjectIds.has(group))
				.map((group) => ({
					type: 'selector' as const,
					id: group
				}))
		];
		accessControlRule.subjects = [...(accessControlRule.subjects ?? []), ...newSubjects];
	}}
/>

<SearchMcpServers
	bind:this={addMcpServerDialog}
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
/>

<Confirm
	msg="Are you sure you want to delete this rule?"
	show={deletingRule}
	onsuccess={async () => {
		if (!accessControlRule.id) return;
		saving = true;
		await AdminService.deleteAccessControlRule(accessControlRule.id);
		goto('/v2/admin/access-control');
	}}
	oncancel={() => (deletingRule = false)}
/>
