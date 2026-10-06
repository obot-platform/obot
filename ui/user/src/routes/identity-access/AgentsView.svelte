<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import ApiKeyRevealDialog from '$lib/components/agent-auth-scope/ApiKeyRevealDialog.svelte';
	import CreateAgentAuthScopeForm from '$lib/components/agent-auth-scope/CreateAgentAuthScopeForm.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import { ApiKeysService, type OrgUser } from '$lib/services';
	import { AUTH_SCOPE_DESCRIPTION } from '$lib/services/api-keys/constants.js';
	import { getAPIKeyCapabilityLabels, type APIKey } from '$lib/services/api-keys/types';
	import { profile } from '$lib/stores';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import { goto, getTableUrlParamsSort, setSortUrlParams } from '$lib/url';
	import { getUserDisplayName, openUrl } from '$lib/utils';
	import { Info, KeyRound, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	interface Props {
		apiKeys: APIKey[];
		users: OrgUser[];
		isAdmin: boolean;
	}

	let { apiKeys: initialApiKeys, users: initialUsers, isAdmin }: Props = $props();
	let apiKeys = $state<APIKey[]>(untrack(() => initialApiKeys));
	let users = $state<OrgUser[]>(untrack(() => initialUsers));

	let deletingKey = $state<APIKey>();
	let loading = $state(false);
	let showCreateNew = $derived(page.url.searchParams.has('new'));
	let createdKeyValue = $state<string>();
	let initSort = $derived(
		getTableUrlParamsSort({ property: isAdmin ? 'userDisplay' : 'name', order: 'asc' })
	);
	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));

	const tableData = $derived(
		apiKeys
			.map((key) => ({
				...key,
				prefix: `ok1-${key.userId}-${key.id}-*****`,
				userDisplay: getUserDisplayName(usersMap, String(key.userId)),
				capabilitiesDisplay: getAPIKeyCapabilityLabels(key),
				createdAtDisplay: formatTimeAgo(key.createdAt).relativeTime,
				lastUsedAtDisplay: key.lastUsedAt
					? formatTimeAgo(key.lastUsedAt).relativeTime
					: m.core_never_used(),
				expiresAtDisplay: key.expiresAt
					? formatTimeUntil(key.expiresAt).relativeTime
					: m.core_never_expires(),
				mcpServerIds: key.mcpServerIds ?? []
			}))
			.filter((key) => (isAdmin ? true : key.userId.toString() === profile.current.id.toString()))
	);

	async function handleDelete() {
		const keyToDelete = deletingKey;
		if (!keyToDelete) return;
		loading = true;
		try {
			await (isAdmin ? ApiKeysService.deleteAnyApiKey : ApiKeysService.deleteApiKey)(
				keyToDelete.id.toString()
			);
			apiKeys = apiKeys.filter((k) => k.id !== keyToDelete.id);
		} finally {
			loading = false;
			deletingKey = undefined;
		}
	}

	async function handleCreate(newKey: APIKey & { key: string }) {
		apiKeys = [newKey, ...apiKeys];
		createdKeyValue = newKey.key;
		hideCreateForm();
	}

	export function showCreateForm() {
		const url = new URL(page.url);
		url.searchParams.set('view', 'agents');
		url.searchParams.set('new', 'true');
		goto(url);
	}

	export function hideCreateForm() {
		const url = new URL(page.url);
		url.searchParams.delete('new');
		goto(url, { replaceState: true });
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

{#if showCreateNew}
	<div
		class="h-full w-full"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<CreateAgentAuthScopeForm onCreate={handleCreate} onCancel={hideCreateForm} />
	</div>
{:else}
	<div class="flex flex-col gap-4">
		{#if isAdmin || apiKeys.length > 0}
			<p class="text-muted-content mb-1 whitespace-pre-line text-sm">
				{AUTH_SCOPE_DESCRIPTION}
			</p>
		{/if}
		{#if apiKeys.length === 0}
			<div class="mt-26 flex w-lg flex-col items-center gap-4 self-center text-center">
				<KeyRound class="text-base-content/80 size-24 opacity-50" />
				<h4 class="text-muted-content text-lg font-semibold">
					{m.identity_access_agents_no_identities()}
				</h4>
				<p class="text-muted-content text-sm font-light">
					{isAdmin
						? m.identity_access_agents_none_in_system()
						: m.identity_access_agents_none_for_you()}
					<br />
					{m.identity_access_agents_click_create()}
				</p>

				{#if !isAdmin}
					<div class="notification-info mt-8">
						<div class="flex flex-col gap-2">
							<div class="flex items-center gap-2">
								<Info class="size-4 shrink-0" />
								<p class="text-sm font-semibold">{m.identity_access_agents_what_are_these()}</p>
							</div>
							<p class="whitespace-pre-line text-left text-sm font-light">
								{AUTH_SCOPE_DESCRIPTION}
								<button class="text-link inline" onclick={showCreateForm}
									>{m.identity_access_agents_create_first()}</button
								>
							</p>
						</div>
					</div>
				{/if}
			</div>
		{:else}
			<Table
				data={tableData}
				fields={isAdmin
					? ['userDisplay', 'name', 'capabilitiesDisplay', 'lastUsedAt', 'expiresAt']
					: ['name', 'capabilitiesDisplay', 'lastUsedAt', 'expiresAt']}
				headers={[
					...(isAdmin ? [{ title: m.core_col_created_by(), property: 'userDisplay' }] : []),
					{ title: m.core_name(), property: 'name' },
					{ title: m.identity_access_agents_col_capabilities(), property: 'capabilitiesDisplay' },
					{ title: m.identity_access_agents_col_last_used(), property: 'lastUsedAt' },
					{ title: m.identity_access_agents_col_expires(), property: 'expiresAt' }
				]}
				filterable={isAdmin ? ['userDisplay', 'name'] : undefined}
				sortable={isAdmin
					? ['userDisplay', 'name', 'lastUsedAt', 'expiresAt']
					: ['lastUsedAt', 'expiresAt']}
				{initSort}
				onSort={setSortUrlParams}
				onClickRow={(d, isCtrlClick) => {
					openUrl(`/identity-access/agents/${d.id}`, isCtrlClick);
				}}
				columnMaxWidths={isAdmin
					? { userDisplay: 200, capabilitiesDisplay: 200, description: 200 }
					: undefined}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'description'}
						<span class="text-muted">{d.description || '-'}</span>
					{:else if property === 'capabilitiesDisplay'}
						{#if d.capabilitiesDisplay.length}
							<div class="flex max-w-48 flex-wrap gap-1 py-1">
								{#each d.capabilitiesDisplay as capability (capability)}
									<span class="badge badge-ghost badge-xs whitespace-nowrap">{capability}</span>
								{/each}
								{#if d.mcpServerIds.length}
									<span class="badge badge-ghost badge-xs whitespace-nowrap"
										>{m.identity_access_agents_servers_badge()}</span
									>
								{/if}
							</div>
						{:else}
							<span class="text-muted">-</span>
						{/if}
					{:else if property === 'lastUsedAt'}
						{d.lastUsedAtDisplay}
					{:else if property === 'expiresAt'}
						{d.expiresAtDisplay}
					{:else if property === 'prefix'}
						<span class="whitespace-nowrap">{d.prefix}</span>
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
				{#snippet actions(d)}
					{@render authScopeActions(d)}
				{/snippet}
			</Table>
		{/if}
	</div>
{/if}

{#snippet authScopeActions(d: APIKey)}
	{@const isOwner = d.userId.toString() === profile.current.id.toString()}
	{@const prefix = `ok1-${d.userId}-${d.id}-*****`}
	{@const url: `/${string}` = `/identity-access/agents/${d.id}/${encodeURIComponent(prefix)}`}
	<DotDotDot classes={{ menu: 'min-w-48 p-0' }}>
		{#if profile.current.hasAdminAccess?.()}
			<div
				class="bg-base-100 dark:bg-base-300 rounded-t-xl pt-2 pb-1 pl-4 text-[11px] font-semibold uppercase"
			>
				{m.identity_access_agents_view_related_logs()}
			</div>
			<div class="flex flex-col gap-1 p-2 bg-base-200">
				<a class="menu-button" href={resolve(url)}>
					{prefix}
				</a>
			</div>
		{/if}
		{#if profile.current.isAdmin?.() || isOwner}
			<div class="flex flex-col gap-1 p-2 pt-1">
				<button class="menu-button text-error" onclick={() => (deletingKey = d)}>
					<Trash2 class="size-4" />
					{m.core_delete()}
				</button>
			</div>
		{/if}
	</DotDotDot>
{/snippet}

<Confirm
	msg={m.identity_access_delete_named_quoted({ name: `${deletingKey?.name}` })}
	show={Boolean(deletingKey)}
	{loading}
	onsuccess={handleDelete}
	oncancel={() => (deletingKey = undefined)}
/>

<ApiKeyRevealDialog keyValue={createdKeyValue} onClose={() => (createdKeyValue = undefined)} />
