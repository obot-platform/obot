<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ApiKeyRevealDialog from '$lib/components/agent-auth-scope/ApiKeyRevealDialog.svelte';
	import CreateAgentAuthScopeForm from '$lib/components/agent-auth-scope/CreateAgentAuthScopeForm.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { ApiKeysService, type OrgUser, type APIKey } from '$lib/services';
	import { AUTH_SCOPE_DESCRIPTION } from '$lib/services/api-keys/constants.js';
	import { getAPIKeyCapabilityLabels } from '$lib/services/api-keys/types';
	import { profile } from '$lib/stores';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import { goto, getTableUrlParamsSort, setSortUrlParams } from '$lib/url';
	import { getUserDisplayName } from '$lib/utils';
	import { openUrl } from '$lib/utils';
	import { KeyRound, Plus, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let allApiKeys = $state<APIKey[]>(untrack(() => data.allApiKeys));
	let users = $state<OrgUser[]>(untrack(() => data.users));

	let deletingKey = $state<APIKey>();
	let loading = $state(false);
	let showCreateNew = $derived(page.url.searchParams.has('new'));
	let createdKeyValue = $state<string>();
	let initSort = $derived(getTableUrlParamsSort({ property: 'userDisplay', order: 'asc' }));

	let usersMap = $derived(new Map(users.map((u) => [u.id, u])));

	const allTableData = $derived(
		allApiKeys.map((key) => ({
			...key,
			prefix: `ok1-${key.userId}-${key.id}-*****`,
			userDisplay: getUserDisplayName(usersMap, String(key.userId)),
			capabilitiesDisplay: getAPIKeyCapabilityLabels(key),
			createdAtDisplay: formatTimeAgo(key.createdAt).relativeTime,
			lastUsedAtDisplay: key.lastUsedAt ? formatTimeAgo(key.lastUsedAt).relativeTime : 'Never',
			expiresAtDisplay: key.expiresAt ? formatTimeUntil(key.expiresAt).relativeTime : 'Never',
			mcpServerIds: key.mcpServerIds ?? []
		}))
	);

	async function handleDeleteAnyKey() {
		const keyToDelete = deletingKey;
		if (!keyToDelete) return;
		loading = true;
		try {
			await ApiKeysService.deleteAnyApiKey(keyToDelete.id.toString());
			allApiKeys = allApiKeys.filter((k) => k.id !== keyToDelete.id);
		} finally {
			loading = false;
			deletingKey = undefined;
		}
	}

	async function handleCreate(newKey: APIKey & { key: string }) {
		allApiKeys = [newKey, ...allApiKeys];
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

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
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
			<CreateAgentAuthScopeForm
				onCreate={handleCreate}
				onCancel={() => goto('/admin/agent-auth-scopes')}
			/>
		</div>
	{:else}
		<div class="flex flex-col gap-4">
			<p class="text-sm">
				{AUTH_SCOPE_DESCRIPTION}
			</p>
			{#if allApiKeys.length === 0}
				<div class="mt-26 flex w-md flex-col items-center gap-4 self-center text-center">
					<KeyRound class="text-muted-content size-24 opacity-50" />
					<h4 class="text-muted-content text-lg font-semibold">No agent auth scopes</h4>
					<p class="text-muted-content text-sm font-light">
						Looks like there aren't any agent auth scopes in the system yet. <br />
						Click the "Create Agent Auth Scope" button above to get started.
					</p>
				</div>
			{:else}
				<Table
					data={allTableData}
					fields={['userDisplay', 'name', 'capabilitiesDisplay', 'lastUsedAt', 'expiresAt']}
					headers={[
						{ title: 'Created By', property: 'userDisplay' },
						{ title: 'Capabilities', property: 'capabilitiesDisplay' },
						{ title: 'Last Used', property: 'lastUsedAt' },
						{ title: 'Expires', property: 'expiresAt' }
					]}
					filterable={['userDisplay', 'name']}
					sortable={['userDisplay', 'name', 'lastUsedAt', 'expiresAt']}
					{initSort}
					onSort={setSortUrlParams}
					onClickRow={(d, isCtrlClick) => {
						const url = `/admin/agent-auth-scopes/${d.id}`;
						openUrl(url, isCtrlClick);
					}}
					columnMaxWidths={{
						userDisplay: 200,
						capabilitiesDisplay: 200,
						description: 200
					}}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'capabilitiesDisplay'}
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
						{#if !isAdminReadonly}
							<DotDotDot>
								<button class="menu-button text-error" onclick={() => (deletingKey = d)}>
									<Trash2 class="size-4" />
									Delete
								</button>
							</DotDotDot>
						{/if}
					{/snippet}
				</Table>
			{/if}
		</div>
	{/if}

	{#snippet rightNavActions()}
		{#if !showCreateNew && !profile.current.isAdminReadonly?.()}
			<button class="btn btn-primary flex items-center gap-2 text-sm" onclick={showCreateForm}>
				<Plus class="size-4" />
				Create Agent Auth Scope
			</button>
		{/if}
	{/snippet}
</Layout>

<Confirm
	msg={`Delete "${deletingKey?.name}"?`}
	show={Boolean(deletingKey)}
	{loading}
	onsuccess={handleDeleteAnyKey}
	oncancel={() => (deletingKey = undefined)}
/>

<ApiKeyRevealDialog keyValue={createdKeyValue} onClose={() => (createdKeyValue = undefined)} />

<svelte:head>
	<title>Obot | Agent Auth Scopes</title>
</svelte:head>
