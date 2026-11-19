<script lang="ts">
	import {
		PAGE_TRANSITION_DURATION,
		DEFAULT_MCP_CATALOG_ID,
		ADMIN_ALL_OPTION,
		MCP_ACCESS_POLICY_FIELD_IDS
	} from '$lib/constants';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		UserService,
		type MCPCatalogServer,
		type AccessControlRule,
		type AccessControlRuleManifest,
		type AccessControlRuleResource,
		type OrgUser,
		type OrgGroup,
		type MCPCatalogEntry
	} from '$lib/services';
	import { getUserRegistry, getMcpValueLabel } from '$lib/services/user/mcp';
	import { profile } from '$lib/stores';
	import { goto } from '$lib/url';
	import { getUserDisplayName } from '$lib/utils';
	import {
		convertSubjectsToTableData,
		resolveSubjects,
		resolveSubjectFromGroup,
		resolveSubjectPickerById
	} from '../../subjectResolver';
	import Confirm from '../Confirm.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import SearchMcpServers from './SearchMcpServers.svelte';
	import SearchUsers from './SearchUsers.svelte';
	import SubjectName from './SubjectName.svelte';
	import { Plus, Trash2 } from '@lucide/svelte';
	import { untrack, type Snippet } from 'svelte';
	import { fly } from 'svelte/transition';

	interface Props {
		topContent?: Snippet;
		accessControlRule?: AccessControlRule;
		onCreate?: (accessControlRule: AccessControlRule) => void;
		onUpdate?: (accessControlRule: AccessControlRule) => void;
		onCancel?: () => void;
		entity?: 'workspace' | 'catalog';
		id?: string | null;
		mcpEntriesContextFn: () => {
			entries: MCPCatalogEntry[];
			servers: MCPCatalogServer[];
			loading: boolean;
		};
		all?: { label: string; description: string };
		readonly?: boolean;
		isAdminView?: boolean;
		animate?: boolean;
	}

	let {
		topContent,
		accessControlRule: initialAccessControlRule,
		onCreate,
		onUpdate,
		onCancel,
		mcpEntriesContextFn,
		readonly,
		isAdminView,
		all = ADMIN_ALL_OPTION,
		id = DEFAULT_MCP_CATALOG_ID,
		entity = 'catalog',
		animate = true
	}: Props = $props();
	const duration = PAGE_TRANSITION_DURATION;
	const noTransition = { duration: 0 };
	const flyRightOut = $derived(animate ? { x: 100, duration } : noTransition);
	const flyRightIn = $derived(animate ? { x: 100, delay: duration } : noTransition);
	const flyLeftOut = $derived(animate ? { x: -100, duration } : noTransition);
	const flyLeftIn = $derived(animate ? { x: -100 } : noTransition);
	let accessControlRule = $state(
		untrack(
			() =>
				initialAccessControlRule ??
				({
					displayName: '',
					userIDs: [],
					mcpServerCatalogEntryNames: [],
					mcpServerNames: []
				} as AccessControlRuleManifest)
		)
	);

	let saving = $state<boolean | undefined>();
	let usersAndGroups = $state<{ users: OrgUser[]; groups: OrgGroup[] }>();
	let loadingUsersAndGroups = $state(false);

	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();
	let addMcpServerDialog = $state<ReturnType<typeof SearchMcpServers>>();

	let deletingRule = $state(false);

	const mcpServerAndEntries = $derived(
		mcpEntriesContextFn?.() ?? {
			entries: [],
			servers: [],
			loading: false
		}
	);
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
			loadingUsersAndGroups = false;
			return;
		}

		loadingUsersAndGroups = true;

		// Groups are resolved by ID, not listed: the directory can hold tens of thousands of them and
		// only the ones attached here are needed.
		const controller = new AbortController();

		resolveSubjects(
			accessControlRule.subjects,
			untrack(() => usersAndGroups),
			{ signal: controller.signal }
		)
			.then((resolved) => {
				if (controller.signal.aborted) return;
				usersAndGroups = resolved;
				loadingUsersAndGroups = false;
			})
			.catch((error) => {
				if (controller.signal.aborted) return;
				console.error('Failed to load users and groups:', error);
				loadingUsersAndGroups = false;
			});

		return () => controller.abort();
	});

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
					name: server?.alias || server?.manifest.name || '-',
					type: 'mcpserver'
				};
			}

			const allLabel = owner
				? isMe
					? m.mcps_access_policies_everything_in_my_registry()
					: m.mcps_access_policies_everything_in_owner_registry({ owner })
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

<div class="flex h-full w-full flex-col gap-4" out:fly={flyRightOut} in:fly={flyRightIn}>
	<div class="flex grow flex-col gap-4" out:fly={flyLeftOut} in:fly={flyLeftIn}>
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
							{@const registry = getUserRegistry(initialAccessControlRule, usersMap)}
							{#if registry}
								<div class="dark:bg-base-300 bg-base-400 rounded-full px-3 py-1 text-xs">
									{getMcpValueLabel(registry)}
								</div>
							{/if}
						{/if}
					{/if}
				</div>
				{#if !readonly}
					<IconButton
						variant="danger2"
						tooltip={{ text: m.mcps_access_policies_delete_catalog() }}
						onclick={() => {
							deletingRule = true;
						}}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			</div>
		{/if}

		{#if !accessControlRule.id}
			<div
				class="dark:bg-base-300 dark:border-base-400 bg-base-100 rounded-lg border border-transparent p-4"
			>
				<div class="flex flex-col gap-6">
					<div class="flex flex-col gap-2">
						<label
							for={MCP_ACCESS_POLICY_FIELD_IDS.name}
							class="flex-1 text-sm font-light capitalize"
						>
							{m.core_name()}
						</label>
						<input
							id={MCP_ACCESS_POLICY_FIELD_IDS.name}
							bind:value={accessControlRule.displayName}
							class="text-input-filled mt-0.5"
							disabled={readonly}
						/>
					</div>
				</div>
			</div>
		{/if}

		<div id={MCP_ACCESS_POLICY_FIELD_IDS.usersGroupsSection} class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">{m.mcps_access_policies_users_groups()}</h2>
				{#if !readonly}
					<div class="relative flex items-center gap-4">
						{#if loadingUsersAndGroups}
							<button
								id={MCP_ACCESS_POLICY_FIELD_IDS.addUserGroupBtn}
								class="btn btn-primary flex items-center gap-1 text-sm"
								disabled
							>
								<Plus class="size-4" />
								{m.core_add_user_group()}
							</button>
						{:else}
							<button
								id={MCP_ACCESS_POLICY_FIELD_IDS.addUserGroupBtn}
								class="btn btn-primary flex items-center gap-1 text-sm"
								onclick={() => {
									addUserGroupDialog?.open();
								}}
							>
								<Plus class="size-4" />
								{m.core_add_user_group()}
							</button>
						{/if}
					</div>
				{/if}
			</div>
			{#if loadingUsersAndGroups}
				<div class="my-2 flex items-center justify-center">
					<Loading class="size-6" />
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
					headers={[
						{ property: 'displayName', title: m.core_name() },
						{ property: 'type', title: m.core_type() }
					]}
					noDataMessage={m.core_no_users_or_groups_added()}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'displayName'}
							<SubjectName name={d.displayName} disabled={d.disabled} />
						{:else}
							{d[property as keyof typeof d]}
						{/if}
					{/snippet}
					{#snippet actions(d)}
						{#if !readonly}
							<IconButton
								variant="danger"
								onclick={() => {
									accessControlRule.subjects = accessControlRule.subjects?.filter(
										(subject) => resolveSubjectPickerById(subject) !== d.id
									);
								}}
								tooltip={{ text: m.core_delete_user_group() }}
							>
								<Trash2 class="size-4" />
							</IconButton>
						{/if}
					{/snippet}
				</Table>
			{/if}
		</div>

		<div id={MCP_ACCESS_POLICY_FIELD_IDS.serversSection} class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">{m.mcps_servers_tab()}</h2>
				{#if !readonly}
					<div class="relative flex items-center gap-4">
						<button
							id={MCP_ACCESS_POLICY_FIELD_IDS.addServerBtn}
							class="btn btn-primary flex items-center gap-1 text-sm"
							onclick={() => {
								addMcpServerDialog?.open();
							}}
						>
							<Plus class="size-4" />
							{m.mcps_access_policies_add_server()}
						</button>
					</div>
				{/if}
			</div>
			<Table
				data={mcpServersTableData}
				fields={['name']}
				noDataMessage={m.mcps_access_policies_no_entries_servers()}
				headers={[{ property: 'name', title: m.core_name() }]}
			>
				{#snippet actions(d)}
					{#if !readonly}
						<IconButton
							variant="danger"
							onclick={() => {
								accessControlRule.resources =
									accessControlRule.resources?.filter((resource) => resource.id !== d.id) ?? [];
							}}
							tooltip={{ text: m.mcps_filters_remove_mcp_server() }}
						>
							<Trash2 class="size-4" />
						</IconButton>
					{/if}
				{/snippet}
			</Table>
		</div>
	</div>
	{#if !readonly}
		<div
			class="bg-base-200 text-muted-content dark:bg-base-100 sticky bottom-0 left-0 flex w-full justify-end gap-2 py-4"
			out:fly={flyLeftOut}
			in:fly={flyLeftIn}
		>
			<div class="flex w-full justify-end gap-4">
				{#if !accessControlRule.id}
					<button
						class="btn btn-secondary"
						onclick={() => {
							if (onCancel) {
								onCancel();
							} else {
								goto('/mcp-servers?view=access-policies');
							}
						}}
					>
						{m.common_cancel()}
					</button>
					<button
						id={MCP_ACCESS_POLICY_FIELD_IDS.saveBtn}
						class="btn btn-primary"
						disabled={!validate(accessControlRule) || saving}
						onclick={async () => {
							if (!id) return;
							saving = true;
							const response =
								entity === 'workspace'
									? await UserService.createWorkspaceAccessControlRule(id, accessControlRule)
									: await AdminService.createAccessControlRule(accessControlRule);
							accessControlRule = response;
							onCreate?.(response);
							saving = false;
						}}
					>
						{#if saving}
							<Loading class="size-4" />
						{:else}
							{m.core_save()}
						{/if}
					</button>
				{:else}
					<button
						class="btn btn-secondary"
						disabled={saving}
						onclick={async () => {
							if (!accessControlRule.id || !id) return;
							saving = true;
							accessControlRule =
								entity === 'workspace'
									? await UserService.getWorkspaceAccessControlRule(id, accessControlRule.id)
									: await AdminService.getAccessControlRule(accessControlRule.id);
							saving = false;
						}}
					>
						{m.core_reset_shared()}
					</button>
					<button
						class="btn btn-primary"
						disabled={!validate(accessControlRule) || saving}
						onclick={async () => {
							if (!accessControlRule.id || !id) return;
							saving = true;
							const response =
								entity === 'workspace'
									? await UserService.updateWorkspaceAccessControlRule(
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
							<Loading class="size-4" />
						{:else}
							{m.core_update()}
						{/if}
					</button>
				{/if}
			</div>
		</div>
	{/if}
</div>

<SearchUsers
	bind:this={addUserGroupDialog}
	filterIds={accessControlRule.subjects?.map(resolveSubjectPickerById) ?? []}
	onAdd={async (users: OrgUser[], groups: OrgGroup[]) => {
		const existingSubjectIds = new Set(
			accessControlRule.subjects?.map(resolveSubjectPickerById) ?? []
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
				.map((group: OrgGroup) => resolveSubjectFromGroup(group))
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
	msg={m.core_delete_named_form({ name: accessControlRule.displayName })}
	show={deletingRule}
	onsuccess={async () => {
		if (!accessControlRule.id || !id) return;
		saving = true;
		await (entity === 'workspace'
			? UserService.deleteWorkspaceAccessControlRule(id, accessControlRule.id)
			: AdminService.deleteAccessControlRule(accessControlRule.id));
		goto('/mcp-servers?view=access-policies');
	}}
	oncancel={() => (deletingRule = false)}
/>
