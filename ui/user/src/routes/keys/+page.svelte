<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { ApiKeysService } from '$lib/services';
	import type { APIKey } from '$lib/services/api-keys/types';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import { Info, KeyRound, Plus, Trash2 } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import CreateApiKeyForm from './CreateApiKeyForm.svelte';
	import ApiKeyRevealDialog from './ApiKeyRevealDialog.svelte';
	import { fly } from 'svelte/transition';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { page } from '$app/state';
	import { goto, getTableUrlParamsSort, setSortUrlParams } from '$lib/url';
	import ServersLabel from '$lib/components/api-keys/ServersLabel.svelte';
	import { openUrl } from '$lib/utils';

	let { data } = $props();
	let apiKeys = $state<APIKey[]>(untrack(() => data.apiKeys));

	let deletingKey = $state<APIKey>();
	let loading = $state(false);
	let showCreateNew = $derived(page.url.searchParams.has('new'));
	let createdKeyValue = $state<string>();
	let initSort = $derived(getTableUrlParamsSort({ property: 'createdAt', order: 'desc' }));

	const tableData = $derived(
		apiKeys.map((key) => ({
			...key,
			prefix: `ok1-${key.userId}-${key.id}-*****`,
			createdAtDisplay: formatTimeAgo(key.createdAt).relativeTime,
			lastUsedAtDisplay: key.lastUsedAt ? formatTimeAgo(key.lastUsedAt).relativeTime : 'Never',
			expiresAtDisplay: key.expiresAt ? formatTimeUntil(key.expiresAt).relativeTime : 'Never',
			mcpServerIds: key.mcpServerIds ?? []
		}))
	);

	async function handleDelete() {
		const keyToDelete = deletingKey;
		if (!keyToDelete) return;
		loading = true;
		try {
			await ApiKeysService.deleteApiKey(keyToDelete.id.toString());
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

<Layout title={showCreateNew ? 'Create API Key' : 'API Keys'} showBackButton={showCreateNew}>
	{#if showCreateNew}
		<div
			class="h-full w-full"
			in:fly={{ x: 100, delay: duration, duration }}
			out:fly={{ x: -100, duration }}
		>
			<CreateApiKeyForm onCreate={handleCreate} onCancel={hideCreateForm} />
		</div>
	{:else}
		<div class="flex flex-col gap-4">
			{#if apiKeys.length === 0}
				<div class="mt-26 flex w-lg flex-col items-center gap-4 self-center text-center">
					<KeyRound class="text-on-surface1 size-24 opacity-50" />
					<h4 class="text-on-surface1 text-lg font-semibold">No API keys</h4>
					<p class="text-on-surface1 text-sm font-light">
						Looks like you don't have any API keys yet! <br />
						Click the "Create API Key" button above to get started.
					</p>

					<div class="notification-info mt-8">
						<div class="flex flex-col gap-2">
							<div class="flex items-center gap-2">
								<Info class="size-4 flex-shrink-0" />
								<p class="text-sm font-semibold">What are these for?</p>
							</div>
							<p class="text-left text-sm font-light">
								API keys allow programmatic access to MCP servers. Each key can only access the
								servers you specify.
								<button class="text-link inline" onclick={showCreateForm}
									>Create your first key</button
								>
							</p>
						</div>
					</div>
				</div>
			{:else}
				<p class="text-muted text-sm">
					API keys allow programmatic access to MCP servers. Each key can only access the servers
					you specify.
				</p>

				<Table
					data={tableData}
					fields={[
						'name',
						'prefix',
						'description',
						'mcpServerIds',
						'createdAt',
						'lastUsedAt',
						'expiresAt'
					]}
					headers={[
						{ title: 'Name', property: 'name' },
						{ title: 'Key', property: 'prefix' },
						{ title: 'Description', property: 'description' },
						{ title: 'Servers', property: 'mcpServerIds' },
						{ title: 'Created', property: 'createdAt' },
						{ title: 'Last Used', property: 'lastUsedAt' },
						{ title: 'Expires', property: 'expiresAt' }
					]}
					sortable={['name', 'createdAt', 'lastUsedAt', 'expiresAt']}
					{initSort}
					onSort={setSortUrlParams}
					onClickRow={(d, isCtrlClick) => {
						const url = `/keys/${d.id}`;
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
						<button class="icon-button" onclick={() => (deletingKey = d)}>
							<Trash2 class="size-4" />
						</button>
					{/snippet}
				</Table>
			{/if}
		</div>
	{/if}

	{#snippet rightNavActions()}
		{#if !showCreateNew}
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
	onsuccess={handleDelete}
	oncancel={() => (deletingKey = undefined)}
/>

<ApiKeyRevealDialog keyValue={createdKeyValue} onClose={() => (createdKeyValue = undefined)} />

<svelte:head>
	<title>Obot | My API Keys</title>
</svelte:head>
