<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { ApiKeysService } from '$lib/services';
	import type { OrgUser } from '$lib/services/admin/types';
	import type { APIKey } from '$lib/services/api-keys/types';
	import { profile } from '$lib/stores';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import { getUserDisplayName } from '$lib/utils';
	import ApiKeyRevealDialog from '../../keys/ApiKeyRevealDialog.svelte';
	import CreateApiKeyDialog from '../../keys/CreateApiKeyDialog.svelte';
	import { Plus, Trash2 } from 'lucide-svelte';
	import { untrack } from 'svelte';

	let { data } = $props();
	let allApiKeys = $state<APIKey[]>(untrack(() => data.allApiKeys));
	let users = $state<OrgUser[]>(untrack(() => data.users));
	let mcpServers = $state(untrack(() => data.mcpServers));

	let deletingKey = $state<APIKey>();
	let loading = $state(false);
	let showCreateDialog = $state(false);
	let createdKeyValue = $state<string>();

	let usersMap = $derived(new Map(users.map((u) => [u.id, u])));

	const allTableData = $derived(
		allApiKeys.map((key) => ({
			...key,
			prefix: `ok1-${key.userId}-${key.id}-*****`,
			userDisplay: getUserDisplayName(usersMap, String(key.userId)),
			createdAtDisplay: formatTimeAgo(key.createdAt).relativeTime,
			lastUsedAtDisplay: key.lastUsedAt ? formatTimeAgo(key.lastUsedAt).relativeTime : 'Never',
			expiresAtDisplay: key.expiresAt ? formatTimeUntil(key.expiresAt).relativeTime : 'Never',
			serverCount: key.mcpServerIds?.length ?? 0
		}))
	);

	async function handleDeleteAnyKey() {
		const keyToDelete = deletingKey;
		if (!keyToDelete) return;
		loading = true;
		try {
			await ApiKeysService.deleteAnyApiKey(keyToDelete.id);
			allApiKeys = allApiKeys.filter((k) => k.id !== keyToDelete.id);
		} finally {
			loading = false;
			deletingKey = undefined;
		}
	}

	async function handleCreate(newKey: APIKey & { key: string }) {
		allApiKeys = [newKey, ...allApiKeys];
		createdKeyValue = newKey.key;
		showCreateDialog = false;
	}

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
</script>

<Layout title="API Keys">
	<div class="flex flex-col gap-4">
		<div class="flex items-center justify-between">
			<p class="text-muted text-sm">View and manage all API keys across all users.</p>
			<button
				class="button-primary flex items-center gap-2"
				onclick={() => (showCreateDialog = true)}
			>
				<Plus class="size-4" />
				Create API Key
			</button>
		</div>

		{#if allApiKeys.length === 0}
			<div class="flex flex-col items-center justify-center gap-4 py-16 text-center">
				<p class="text-muted">No API keys exist in the system.</p>
			</div>
		{:else}
			<Table
				data={allTableData}
				fields={[
					'userDisplay',
					'name',
					'prefix',
					'description',
					'serverCount',
					'createdAtDisplay',
					'lastUsedAtDisplay',
					'expiresAtDisplay'
				]}
				headers={[
					{ title: 'User', property: 'userDisplay' },
					{ title: 'Name', property: 'name' },
					{ title: 'Key', property: 'prefix' },
					{ title: 'Description', property: 'description' },
					{ title: 'Servers', property: 'serverCount' },
					{ title: 'Created', property: 'createdAtDisplay' },
					{ title: 'Last Used', property: 'lastUsedAtDisplay' },
					{ title: 'Expires', property: 'expiresAtDisplay' }
				]}
				filterable={['userDisplay', 'name']}
				sortable={[
					'userDisplay',
					'name',
					'createdAtDisplay',
					'lastUsedAtDisplay',
					'expiresAtDisplay'
				]}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'description'}
						<span class="text-muted">{d.description || '-'}</span>
					{:else if property === 'serverCount'}
						<span class="pill-rounded bg-surface3">
							{d.serverCount} server{d.serverCount === 1 ? '' : 's'}
						</span>
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
				{#snippet actions(d)}
					{#if !isAdminReadonly}
						<DotDotDot>
							<div class="default-dialog flex min-w-max flex-col p-2">
								<button class="menu-button text-red-500" onclick={() => (deletingKey = d)}>
									<Trash2 class="size-4" />
									Delete
								</button>
							</div>
						</DotDotDot>
					{/if}
				{/snippet}
			</Table>
		{/if}
	</div>
</Layout>

<Confirm
	msg={`Are you sure you want to delete API key "${deletingKey?.name}"? This action cannot be undone.`}
	show={Boolean(deletingKey)}
	{loading}
	onsuccess={handleDeleteAnyKey}
	oncancel={() => (deletingKey = undefined)}
/>

<CreateApiKeyDialog bind:show={showCreateDialog} {mcpServers} onCreate={handleCreate} />

<ApiKeyRevealDialog keyValue={createdKeyValue} onClose={() => (createdKeyValue = undefined)} />
