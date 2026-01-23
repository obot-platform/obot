<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { ApiKeysService } from '$lib/services';
	import type { APIKey } from '$lib/services/api-keys/types';
	import type { OrgUser } from '$lib/services/admin/types';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import { profile } from '$lib/stores';
	import { getUserDisplayName } from '$lib/utils';
	import { KeyRound, Plus, Trash2 } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import ApiKeyRevealDialog from '../../keys/ApiKeyRevealDialog.svelte';
	import CreateApiKeyForm from '../../keys/CreateApiKeyForm.svelte';
	import { fly } from 'svelte/transition';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { page } from '$app/state';
	import { goto, getTableUrlParamsSort, setSortUrlParams } from '$lib/url';
	import ServersLabel from '$lib/components/api-keys/ServersLabel.svelte';
	import { openUrl } from '$lib/utils';

	let { data } = $props();
	let allApiKeys = $state<APIKey[]>(untrack(() => data.allApiKeys));
	let users = $state<OrgUser[]>(untrack(() => data.users));

	let deletingKey = $state<APIKey>();
	let loading = $state(false);
	let showCreateNew = $derived(page.url.searchParams.has('new'));
	let createdKeyValue = $state<string>();
	let initSort = $derived(getTableUrlParamsSort({ property: 'createdAt', order: 'desc' }));

	let usersMap = $derived(new Map(users.map((u) => [u.id, u])));

	const allTableData = $derived(
		allApiKeys.map((key) => ({
			...key,
			prefix: `ok1-${key.userId}-${key.id}-*****`,
			userDisplay: getUserDisplayName(usersMap, String(key.userId)),
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

<Layout title={showCreateNew ? 'Create API Key' : 'API Keys'} showBackButton={showCreateNew}>
	{#if showCreateNew}
		<div
			class="h-full w-full"
			in:fly={{ x: 100, delay: duration, duration }}
			out:fly={{ x: -100, duration }}
		>
			<CreateApiKeyForm onCreate={handleCreate} onCancel={() => (showCreateNew = false)} />
		</div>
	{:else}
		<div class="flex flex-col gap-4">
			{#if allApiKeys.length === 0}
				<div class="mt-26 flex w-md flex-col items-center gap-4 self-center text-center">
					<KeyRound class="text-on-surface1 size-24 opacity-50" />
					<h4 class="text-on-surface1 text-lg font-semibold">No API keys</h4>
					<p class="text-on-surface1 text-sm font-light">
						Looks like there aren't any API keys in the system yet. <br />
						Click the "Create API Key" button above to get started.
					</p>
				</div>
			{:else}
				<p class="text-muted text-sm">View and manage all API keys across all users.</p>
				<Table
					data={allTableData}
					fields={[
						'userDisplay',
						'name',
						'prefix',
						'description',
						'mcpServerIds',
						'createdAt',
						'lastUsedAt',
						'expiresAt'
					]}
					headers={[
						{ title: 'User', property: 'userDisplay' },
						{ title: 'Name', property: 'name' },
						{ title: 'Key', property: 'prefix' },
						{ title: 'Description', property: 'description' },
						{ title: 'Servers', property: 'mcpServerIds' },
						{ title: 'Created', property: 'createdAt' },
						{ title: 'Last Used', property: 'lastUsedAt' },
						{ title: 'Expires', property: 'expiresAt' }
					]}
					filterable={['userDisplay', 'name']}
					sortable={['userDisplay', 'name', 'createdAt', 'lastUsedAt', 'expiresAt']}
					{initSort}
					onSort={setSortUrlParams}
					onClickRow={(d, isCtrlClick) => {
						const url = `/admin/api-keys/${d.id}`;
						openUrl(url, isCtrlClick);
					}}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'description'}
							<span class="text-muted">{d.description || '-'}</span>
						{:else if property === 'mcpServerIds'}
							<ServersLabel mcpServerIds={d.mcpServerIds} />
						{:else if property === 'createdAt'}
							{d.createdAtDisplay}
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
								<button class="menu-button text-red-500" onclick={() => (deletingKey = d)}>
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
			<button class="button-primary flex items-center gap-2" onclick={showCreateForm}>
				<Plus class="size-4" />
				Create API Key
			</button>
		{/if}
	{/snippet}
</Layout>

<Confirm
	msg={`Delete API key "${deletingKey?.name}"?`}
	show={Boolean(deletingKey)}
	{loading}
	onsuccess={handleDeleteAnyKey}
	oncancel={() => (deletingKey = undefined)}
/>

<ApiKeyRevealDialog keyValue={createdKeyValue} onClose={() => (createdKeyValue = undefined)} />

<svelte:head>
	<title>Obot | API Keys</title>
</svelte:head>
