<script lang="ts">
	import { browser } from '$app/environment';
	import { afterNavigate, goto } from '$app/navigation';
	import AuditDetails from '$lib/components/admin/AuditDetails.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { type OrgUser, type AuditLogFilters, AdminService } from '$lib/services';
	import { Captions, X } from 'lucide-svelte';
	import { fade } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let users = $state<OrgUser[]>([]);
	let currentFilters = $state<AuditLogFilters & { mcpId?: string | null }>({});

	afterNavigate(() => {
		currentFilters = compileFilters();

		AdminService.listUsers().then((userData) => {
			users = userData;
		});
	});

	function compileFilters(): AuditLogFilters & { mcpId?: string | null } {
		if (!browser) return {};

		const url = new URL(window.location.href);
		const mcpId = url.searchParams.get('mcpId');
		const startTime = url.searchParams.get('startTime')
			? decodeURIComponent(url.searchParams.get('startTime')!)
			: null;
		const endTime = url.searchParams.get('endTime')
			? decodeURIComponent(url.searchParams.get('endTime')!)
			: null;
		const userId = url.searchParams.get('userId');
		const client = url.searchParams.get('client')
			? decodeURIComponent(url.searchParams.get('client')!)
			: null;
		const callType = url.searchParams.get('callType');
		const sessionId = url.searchParams.get('sessionId');
		const mcpServerDisplayName = url.searchParams.get('name')
			? decodeURIComponent(url.searchParams.get('name')!)
			: null;
		const mcpServerCatalogEntryName = url.searchParams.get('entryId')
			? decodeURIComponent(url.searchParams.get('entryId')!)
			: null;

		return {
			mcpId,
			startTime,
			endTime,
			userId,
			client,
			callType,
			sessionId,
			mcpServerDisplayName,
			mcpServerCatalogEntryName
		};
	}

	function convertFilterDisplayLabel(key: string) {
		if (key === 'mcpServerDisplayName') return 'Server';
		if (key === 'mcpServerCatalogEntryName') return 'Server ID';
		if (key === 'mcpId') return 'Server ID';
		if (key === 'startTime') return 'Start Time';
		if (key === 'endTime') return 'End Time';
		if (key === 'userId') return 'User ID';
		if (key === 'client') return 'Client';
		if (key === 'callType') return 'Call Type';
		if (key === 'sessionId') return 'Session ID';
		return key;
	}
</script>

<Layout>
	<div class="my-4 h-screen" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex min-h-full flex-col gap-8 pb-8">
			<div class="flex items-center justify-between gap-4">
				<h1 class="text-2xl font-semibold">Audit Logs</h1>
			</div>
			{@render filters()}
			{@render logsContent()}
		</div>
	</div>
</Layout>

{#snippet filters()}
	{@const keys = Object.keys(currentFilters)}
	{@const hasFilters = Object.entries(currentFilters).some(([_, value]) => value)}
	{#if hasFilters}
		<div class="flex flex-wrap items-center gap-2">
			{#each keys as key (key)}
				{@const value = currentFilters[key as keyof typeof currentFilters]}
				{#if value}
					<div
						class="flex items-center gap-1 rounded-full border border-blue-500 bg-blue-500/33 px-4 py-2"
					>
						<p class="text-xs font-semibold">
							{convertFilterDisplayLabel(key)}: <span class="font-light">{value}</span>
						</p>

						<button
							class="rounded-full p-1 transition-colors duration-200 hover:bg-blue-500/50"
							onclick={() => {
								const url = new URL(window.location.href);

								let urlKey = key;
								if (key === 'mcpServerDisplayName') {
									urlKey = 'name';
								} else if (key === 'mcpServerCatalogEntryName') {
									urlKey = 'entryId';
								}
								url.searchParams.delete(urlKey);
								goto(url.toString());
							}}
						>
							<X class="size-3" />
						</button>
					</div>
				{/if}
			{/each}
		</div>
	{/if}
{/snippet}

{#snippet logsContent()}
	{@const { mcpId, mcpServerCatalogEntryName, mcpServerDisplayName } = currentFilters}
	<div class="flex flex-col gap-8" in:fade={{ duration }}>
		<AuditDetails
			allowPagination
			mcpId={mcpId ?? undefined}
			mcpCatalogEntryId={mcpServerCatalogEntryName ?? undefined}
			mcpServerDisplayName={mcpServerDisplayName ?? undefined}
			{users}
		>
			{#snippet emptyContent()}
				<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<Captions class="size-24 text-gray-200 dark:text-gray-900" />
					<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No audit logs</h4>
					<p class="text-sm font-light text-gray-400 dark:text-gray-600">
						Currently, there are no audit logs.
					</p>
				</div>
			{/snippet}
		</AuditDetails>
	</div>
{/snippet}

<svelte:head>
	<title>Obot | Audit Logs</title>
</svelte:head>
