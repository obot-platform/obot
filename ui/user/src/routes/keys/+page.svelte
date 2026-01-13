<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { ApiKeysService } from '$lib/services';
	import type { APIKey } from '$lib/services/api-keys/types';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import ApiKeyRevealDialog from './ApiKeyRevealDialog.svelte';
	import CreateApiKeyDialog from './CreateApiKeyDialog.svelte';
	import { Plus, Trash2 } from 'lucide-svelte';
	import { untrack } from 'svelte';

	let { data } = $props();
	let apiKeys = $state<APIKey[]>(untrack(() => data.apiKeys));
	let mcpServers = $state(untrack(() => data.mcpServers));

	let deletingKey = $state<APIKey>();
	let loading = $state(false);
	let showCreateDialog = $state(false);
	let createdKeyValue = $state<string>();

	const tableData = $derived(
		apiKeys.map((key) => ({
			...key,
			prefix: `ok1-${key.userId}-${key.id}-*****`,
			createdAtDisplay: formatTimeAgo(key.createdAt).relativeTime,
			lastUsedAtDisplay: key.lastUsedAt ? formatTimeAgo(key.lastUsedAt).relativeTime : 'Never',
			expiresAtDisplay: key.expiresAt ? formatTimeUntil(key.expiresAt).relativeTime : 'Never',
			serverCount: key.mcpServerIds?.length ?? 0
		}))
	);

	async function handleDelete() {
		const keyToDelete = deletingKey;
		if (!keyToDelete) return;
		loading = true;
		try {
			await ApiKeysService.deleteApiKey(keyToDelete.id);
			apiKeys = apiKeys.filter((k) => k.id !== keyToDelete.id);
		} finally {
			loading = false;
			deletingKey = undefined;
		}
	}

	async function handleCreate(newKey: APIKey & { key: string }) {
		apiKeys = [newKey, ...apiKeys];
		createdKeyValue = newKey.key;
		showCreateDialog = false;
	}
</script>

<Layout title="API Keys">
	<div class="flex flex-col gap-4">
		<div class="flex items-center justify-between">
			<p class="text-muted text-sm">
				API keys allow programmatic access to MCP servers. Each key can only access the servers you
				specify.
			</p>
			<button
				class="button-primary flex items-center gap-2"
				onclick={() => (showCreateDialog = true)}
			>
				<Plus class="size-4" />
				Create API Key
			</button>
		</div>

		{#if apiKeys.length === 0}
			<div class="flex flex-col items-center justify-center gap-4 py-16 text-center">
				<p class="text-muted">No API keys yet.</p>
				<button
					class="button-primary flex items-center gap-2"
					onclick={() => (showCreateDialog = true)}
				>
					<Plus class="size-4" />
					Create your first API key
				</button>
			</div>
		{:else}
			<Table
				data={tableData}
				fields={[
					'name',
					'prefix',
					'description',
					'serverCount',
					'createdAtDisplay',
					'lastUsedAtDisplay',
					'expiresAtDisplay'
				]}
				headers={[
					{ title: 'Name', property: 'name' },
					{ title: 'Key', property: 'prefix' },
					{ title: 'Description', property: 'description' },
					{ title: 'Servers', property: 'serverCount' },
					{ title: 'Created', property: 'createdAtDisplay' },
					{ title: 'Last Used', property: 'lastUsedAtDisplay' },
					{ title: 'Expires', property: 'expiresAtDisplay' }
				]}
				sortable={['name', 'createdAtDisplay', 'lastUsedAtDisplay', 'expiresAtDisplay']}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'description'}
						<span class="text-muted">{d.description || '-'}</span>
					{:else if property === 'serverCount'}
						<span class="rounded bg-gray-100 px-2 py-0.5 text-xs dark:bg-gray-800">
							{d.serverCount} server{d.serverCount === 1 ? '' : 's'}
						</span>
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
				{#snippet actions(d)}
					<DotDotDot>
						<div class="default-dialog flex min-w-max flex-col p-2">
							<button class="menu-button text-red-500" onclick={() => (deletingKey = d)}>
								<Trash2 class="size-4" />
								Delete
							</button>
						</div>
					</DotDotDot>
				{/snippet}
			</Table>
		{/if}
	</div>
</Layout>

<Confirm
	msg={`Are you sure you want to delete API key "${deletingKey?.name}"? This action cannot be undone.`}
	show={Boolean(deletingKey)}
	{loading}
	onsuccess={handleDelete}
	oncancel={() => (deletingKey = undefined)}
/>

<CreateApiKeyDialog bind:show={showCreateDialog} {mcpServers} onCreate={handleCreate} />

<ApiKeyRevealDialog keyValue={createdKeyValue} onClose={() => (createdKeyValue = undefined)} />
