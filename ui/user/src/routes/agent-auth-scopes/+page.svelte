<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ApiKeyRevealDialog from '$lib/components/agent-auth-scope/ApiKeyRevealDialog.svelte';
	import CreateAgentAuthScopeForm from '$lib/components/agent-auth-scope/CreateAgentAuthScopeForm.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { ApiKeysService, type OrgUser } from '$lib/services';
	import { AUTH_SCOPE_DESCRIPTION } from '$lib/services/api-keys/constants.js';
	import { getAPIKeyCapabilityLabels, type APIKey } from '$lib/services/api-keys/types';
	import { profile } from '$lib/stores';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import { goto, getTableUrlParamsSort, setSortUrlParams } from '$lib/url';
	import { getUserDisplayName, openUrl } from '$lib/utils';
	import { Info, KeyRound, Plus, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let apiKeys = $state<APIKey[]>(untrack(() => data.apiKeys));
	let users = $state<OrgUser[]>(untrack(() => data.users));
	let isAdminPage = $derived(data.isAdmin);

	let deletingKey = $state<APIKey>();
	let loading = $state(false);
	let showCreateNew = $derived(page.url.searchParams.has('new'));
	let createdKeyValue = $state<string>();
	let initSort = $derived(
		getTableUrlParamsSort({ property: isAdminPage ? 'userDisplay' : 'name', order: 'asc' })
	);
	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));
	let isAdminReadonly = $derived(isAdminPage && profile.current.isAdminReadonly?.());

	const tableData = $derived(
		apiKeys
			.map((key) => ({
				...key,
				prefix: `ok1-${key.userId}-${key.id}-*****`,
				userDisplay: getUserDisplayName(usersMap, String(key.userId)),
				capabilitiesDisplay: getAPIKeyCapabilityLabels(key),
				createdAtDisplay: formatTimeAgo(key.createdAt).relativeTime,
				lastUsedAtDisplay: key.lastUsedAt ? formatTimeAgo(key.lastUsedAt).relativeTime : 'Never',
				expiresAtDisplay: key.expiresAt ? formatTimeUntil(key.expiresAt).relativeTime : 'Never',
				mcpServerIds: key.mcpServerIds ?? []
			}))
			.filter((key) =>
				isAdminPage ? true : key.userId.toString() === profile.current.id.toString()
			)
	);

	async function handleDelete() {
		const keyToDelete = deletingKey;
		if (!keyToDelete) return;
		loading = true;
		try {
			await (isAdminPage ? ApiKeysService.deleteAnyApiKey : ApiKeysService.deleteApiKey)(
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

	function showCreateForm() {
		const url = new URL(page.url);
		url.searchParams.set('new', 'true');
		goto(url);
	}

	function hideCreateForm() {
		const url = new URL(page.url);
		url.searchParams.delete('new');
		goto(url, { replaceState: true });
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout
	title={showCreateNew ? 'Create Agent Auth Scope' : 'Agent Auth Scopes'}
	showBackButton={showCreateNew}
>
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
			{#if isAdminPage || apiKeys.length > 0}
				<p class="text-muted-content mb-1 whitespace-pre-line text-sm">
					{AUTH_SCOPE_DESCRIPTION}
				</p>
			{/if}
			{#if apiKeys.length === 0}
				<div class="mt-26 flex w-lg flex-col items-center gap-4 self-center text-center">
					<KeyRound class="text-base-content/80 size-24 opacity-50" />
					<h4 class="text-muted-content text-lg font-semibold">No agent auth scopes</h4>
					<p class="text-muted-content text-sm font-light">
						{isAdminPage
							? "Looks like there aren't any agent auth scopes in the system yet."
							: "Looks like you don't have any agent auth scopes yet!"}
						<br />
						Click the "Create Agent Auth Scope" button above to get started.
					</p>

					{#if !isAdminPage}
						<div class="notification-info mt-8">
							<div class="flex flex-col gap-2">
								<div class="flex items-center gap-2">
									<Info class="size-4 shrink-0" />
									<p class="text-sm font-semibold">What are these for?</p>
								</div>
								<p class="whitespace-pre-line text-left text-sm font-light">
									{AUTH_SCOPE_DESCRIPTION}
									<button class="text-link inline" onclick={showCreateForm}
										>Create your first auth scope</button
									>
								</p>
							</div>
						</div>
					{/if}
				</div>
			{:else}
				<Table
					data={tableData}
					fields={isAdminPage
						? ['userDisplay', 'name', 'capabilitiesDisplay', 'lastUsedAt', 'expiresAt']
						: ['name', 'capabilitiesDisplay', 'lastUsedAt', 'expiresAt']}
					headers={[
						...(isAdminPage ? [{ title: 'Created By', property: 'userDisplay' }] : []),
						{ title: 'Capabilities', property: 'capabilitiesDisplay' },
						{ title: 'Last Used', property: 'lastUsedAt' },
						{ title: 'Expires', property: 'expiresAt' }
					]}
					filterable={isAdminPage ? ['userDisplay', 'name'] : undefined}
					sortable={isAdminPage
						? ['userDisplay', 'name', 'lastUsedAt', 'expiresAt']
						: ['lastUsedAt', 'expiresAt']}
					{initSort}
					onSort={setSortUrlParams}
					onClickRow={(d, isCtrlClick) => {
						const url = `${isAdminPage ? '/admin' : ''}/agent-auth-scopes/${d.id}`;
						openUrl(url, isCtrlClick);
					}}
					columnMaxWidths={isAdminPage
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
										<span class="badge badge-ghost badge-xs whitespace-nowrap">Servers</span>
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

	{#snippet rightNavActions()}
		{#if !showCreateNew && !isAdminReadonly}
			<button class="btn btn-primary flex items-center gap-2 text-sm" onclick={showCreateForm}>
				<Plus class="size-4" />
				Create Agent Auth Scope
			</button>
		{/if}
	{/snippet}
</Layout>

{#snippet authScopeActions(d: APIKey)}
	{@const isOwner = d.userId.toString() === profile.current.id.toString()}
	{@const prefix = `ok1-${d.userId}-${d.id}-*****`}
	{@const url: `/${string}` = isAdminPage
		? `/admin/agent-auth-scopes/${d.id}/${encodeURIComponent(prefix)}`
		: `/agent-auth-scopes/${d.id}/${encodeURIComponent(prefix)}`}
	<DotDotDot classes={{ menu: 'min-w-48 p-0' }}>
		{#if profile.current.hasAdminAccess?.()}
			<div
				class="bg-base-100 dark:bg-base-300 rounded-t-xl pt-2 pb-1 pl-4 text-[11px] font-semibold uppercase"
			>
				View Related Logs
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
					Delete
				</button>
			</div>
		{/if}
	</DotDotDot>
{/snippet}

<Confirm
	msg={`Delete "${deletingKey?.name}"?`}
	show={Boolean(deletingKey)}
	{loading}
	onsuccess={handleDelete}
	oncancel={() => (deletingKey = undefined)}
/>

<ApiKeyRevealDialog keyValue={createdKeyValue} onClose={() => (createdKeyValue = undefined)} />

<svelte:head>
	<title>Obot | {isAdminPage ? 'Agent Auth Scopes' : 'Agent Auth Keys'}</title>
</svelte:head>
