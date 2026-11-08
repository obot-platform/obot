<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import BuiltinMCPSetup from '$lib/components/mcp-server-requests/BuiltinMCPSetup.svelte';
	import RequestStateBadge from '$lib/components/mcp-server-requests/RequestStateBadge.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import {
		listMCPServerRequests,
		requestTarget,
		type MCPServerRequest,
		type MCPServerRequestState
	} from '$lib/services/mcpserverrequests';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { Inbox } from '@lucide/svelte';

	type Filter = MCPServerRequestState | 'all';

	let isAdmin = $derived(!!profile.current.isAdmin?.());
	let filter = $state<Filter>('pending');
	let requests = $state<MCPServerRequest[]>([]);
	let loading = $state(true);
	let error = $state('');
	let generation = 0;

	const filters: { value: Filter; label: () => string }[] = [
		{ value: 'pending', label: m.server_requests_state_pending },
		{ value: 'approved', label: m.server_requests_state_approved },
		{ value: 'denied', label: m.server_requests_state_denied },
		{ value: 'all', label: m.server_requests_state_all }
	];

	async function load() {
		const gen = ++generation;
		loading = true;
		error = '';
		try {
			const items = await listMCPServerRequests(
				{ state: filter === 'all' ? undefined : filter },
				{ dontLogErrors: true }
			);
			if (gen === generation) requests = items;
		} catch (e) {
			if (gen === generation) error = parseErrorContent(e).message;
		} finally {
			if (gen === generation) loading = false;
		}
	}

	$effect(() => {
		void filter;
		load();
	});
</script>

<svelte:head>
	<title>Obot | {m.server_requests_title()}</title>
</svelte:head>

<Layout title={m.server_requests_title()}>
	<div class="flex flex-col gap-6 pb-8">
		<p class="text-muted-content text-sm">
			{isAdmin ? m.server_requests_description_admin() : m.server_requests_description_user()}
		</p>

		<BuiltinMCPSetup admin={isAdmin} />

		<div class="flex flex-wrap gap-2" role="group" aria-label={m.server_requests_title()}>
			{#each filters as f (f.value)}
				<button
					type="button"
					class="btn rounded-4xl text-sm {filter === f.value ? 'btn-primary' : 'btn-secondary'}"
					aria-pressed={filter === f.value}
					onclick={() => (filter = f.value)}
				>
					{f.label()}
				</button>
			{/each}
		</div>

		{#if error}
			<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
		{/if}

		{#if loading && requests.length === 0}
			<Skeleton type="card" class="min-h-24 w-full" />
		{:else if requests.length === 0}
			<div class="mt-8 flex flex-col items-center gap-4 self-center text-center">
				<Inbox class="text-muted-content size-20 opacity-25" />
				<p class="text-muted-content text-sm">{m.server_requests_empty()}</p>
			</div>
		{:else}
			<ul class="flex flex-col gap-3">
				{#each requests as request (request.id)}
					<li>
						<a
							href="/mcp-server-requests/{request.id}"
							class="dark:bg-base-200 dark:border-base-400 bg-base-100 hover:border-primary flex flex-col gap-2 rounded-lg border border-transparent p-4 shadow-sm transition-colors"
						>
							<div class="flex flex-wrap items-center justify-between gap-2">
								<span class="font-semibold">{request.name}</span>
								<RequestStateBadge state={request.state} />
							</div>
							<code class="text-muted-content truncate text-xs">{requestTarget(request)}</code>
							{#if request.reason}
								<p class="line-clamp-2 text-sm">{request.reason}</p>
							{/if}
							<p class="text-muted-content text-xs">
								{#if isAdmin}
									{m.server_requests_requested_by({
										name: request.requesterName || request.requesterEmail || request.requesterID
									})} ·
								{/if}
								{m.server_requests_requested_at({
									time: formatTimeAgo(request.created).relativeTime
								})}
							</p>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</Layout>
