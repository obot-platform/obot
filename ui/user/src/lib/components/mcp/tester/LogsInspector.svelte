<script lang="ts">
	import JsonPreview from '$lib/components/JsonPreview.svelte';
	import Select from '$lib/components/Select.svelte';
	import { formatFileSize } from '$lib/format';
	import { m } from '$lib/i18n';
	import type { TesterLogEntry, TesterLogKind } from '$lib/services/mcp/tester-log.svelte';
	import type { MCPTesterSession, TesterStatus } from '$lib/services/mcp/tester.svelte';
	import CornerCopyButton from './CornerCopyButton.svelte';
	import {
		Check,
		ChevronRight,
		Copy,
		RotateCw,
		Search,
		Trash2,
		TriangleAlert
	} from '@lucide/svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		session?: MCPTesterSession;
		serverName: string;
		onretry?: () => void;
	}

	let { session, serverName, onretry }: Props = $props();

	const KIND_BADGE: Record<TesterLogKind, string> = {
		request: 'badge-ghost',
		response: 'badge-success',
		'error-response': 'badge-error',
		notification: 'badge-info',
		'transport-event': 'badge-warning'
	};
	const KIND_LABEL: Record<TesterLogKind, string> = {
		request: m.mcps_tester_log_kind_request(),
		response: m.mcps_tester_log_kind_response(),
		'error-response': m.mcps_tester_log_kind_error(),
		notification: m.mcps_tester_log_kind_notification(),
		'transport-event': m.mcps_tester_log_kind_transport()
	};
	const STATUS_LABEL: Record<TesterStatus, string> = {
		idle: m.mcps_tester_log_status_idle(),
		initializing: m.mcps_tester_log_status_initializing(),
		ready: m.core_mcp_value_connected(),
		'access-denied': m.mcps_tester_access_denied(),
		unhealthy: m.mcps_tester_server_unavailable(),
		'reauthentication-required': m.mcps_tester_reauth_required(),
		'setup-required': m.mcps_tester_status_setup_required(),
		error: m.mcps_servers_connection_failed(),
		closed: m.mcps_tester_log_status_closed()
	};
	const KIND_OPTIONS: Array<{ id: 'all' | TesterLogKind; label: string }> = [
		{ id: 'all', label: m.mcps_tester_log_filter_all() },
		{ id: 'request', label: m.mcps_tester_log_filter_requests() },
		{ id: 'response', label: m.mcps_tester_log_filter_responses() },
		{ id: 'error-response', label: m.mcps_tester_log_filter_errors() },
		{ id: 'notification', label: m.mcps_tester_log_filter_notifications() },
		{ id: 'transport-event', label: m.mcps_tester_log_filter_transport() }
	];
	const MAX_INLINE_JSON = 256 * 1024;
	const SUMMARY_LIMIT = 140;

	const summaries = new WeakMap<object, string>();
	const rawJSON = new WeakMap<object, string>();

	let query = $state('');
	let kindFilter = $state<'all' | TesterLogKind>('all');
	let follow = $state(true);
	let listElement = $state<HTMLElement>();
	let expanded = new SvelteSet<number>();
	let forceRender = new SvelteSet<number>();
	let announcement = $state('');
	let copied = $state(false);
	let lastAnnounced = 0;

	let entries = $derived(session?.log.entries ?? []);
	let dropped = $derived(session?.log.dropped ?? 0);
	let status = $derived(session?.status);

	let filtered = $derived.by(() => {
		const q = query.trim().toLocaleLowerCase();
		// Returning the same array on the common no-filter path avoids reallocating
		// (and re-keying) on every append.
		if (kindFilter === 'all' && !q) return entries;
		return entries.filter(
			(entry) =>
				(kindFilter === 'all' || entry.kind === kindFilter) &&
				(!q ||
					(entry.method ?? '').toLocaleLowerCase().includes(q) ||
					String(entry.rpcId ?? '').includes(q))
		);
	});

	let ndjson = $derived(session?.log.toNDJSON(filtered) ?? '');
	let connectionFailed = $derived(
		status === 'error' || status === 'unhealthy' || status === 'access-denied'
	);

	function clock(at: number): string {
		const date = new Date(at);
		return `${date.toLocaleTimeString(undefined, { hour12: false })}.${String(date.getMilliseconds()).padStart(3, '0')}`;
	}

	function buildSummary(entry: TesterLogEntry): string {
		if (entry.kind === 'transport-event') {
			if (entry.event === 'error') return entry.errorMessage ?? m.mcps_tester_log_transport_error();
			return entry.event === 'connecting'
				? m.mcps_tester_connecting_to({ name: serverName })
				: m.mcps_tester_log_connection_closed();
		}
		const message = entry.message as Record<string, unknown> | undefined;
		if (!message) return '';
		const error = message.error as { code?: unknown; message?: unknown } | undefined;
		if (error) return `${error.code ?? 'error'}: ${error.message ?? ''}`;
		const params = message.params as Record<string, unknown> | undefined;
		if (params) {
			return typeof params.name === 'string' ? params.name : Object.keys(params).join(', ');
		}
		const result = message.result as Record<string, unknown> | undefined;
		if (result) return Object.keys(result).join(', ') || 'ok';
		return '';
	}

	function summarize(entry: TesterLogEntry): string {
		const cached = summaries.get(entry);
		if (cached !== undefined) return cached;
		const value = buildSummary(entry).slice(0, SUMMARY_LIMIT);
		summaries.set(entry, value);
		return value;
	}

	function rawFor(entry: TesterLogEntry): string {
		const cached = rawJSON.get(entry);
		if (cached !== undefined) return cached;
		let value: string;
		try {
			value = JSON.stringify(entry.message ?? entry, null, 2) ?? '';
		} catch {
			value = m.mcps_tester_log_unserializable();
		}
		rawJSON.set(entry, value);
		return value;
	}

	function toggle(id: number) {
		if (expanded.has(id)) {
			expanded.delete(id);
			return;
		}
		expanded.add(id);
		follow = false;
	}

	function handleScroll() {
		if (!listElement) return;
		follow =
			Math.abs(listElement.scrollHeight - listElement.clientHeight - listElement.scrollTop) < 16;
	}

	async function copyLog() {
		try {
			await navigator.clipboard.writeText(ndjson);
			copied = true;
			setTimeout(() => (copied = false), 750);
		} catch {
			copied = false;
		}
	}

	function clearFilters() {
		query = '';
		kindFilter = 'all';
	}

	function clearLog() {
		session?.log.clear();
		expanded.clear();
		forceRender.clear();
	}

	$effect(() => {
		const tail = `${entries.at(-1)?.id ?? ''}:${filtered.length}`;
		void tail;
		if (!follow || !listElement) return;
		const element = listElement;
		const frame = requestAnimationFrame(() => {
			element.scrollTo({ top: element.scrollHeight, behavior: 'auto' });
		});
		return () => cancelAnimationFrame(frame);
	});

	$effect(() => {
		const count = entries.length;
		if (!follow) return;
		const now = Date.now();
		if (now - lastAnnounced < 2000) return;
		lastAnnounced = now;
		announcement =
			count === 1
				? m.mcps_tester_log_entries_one({ count })
				: m.mcps_tester_log_entries_other({ count });
	});
</script>

<div class="flex h-full min-h-0 flex-col">
	<h2 class="sr-only">{m.mcps_tester_mcp_log()}</h2>

	{#if session && status && status !== 'ready' && status !== 'idle'}
		<div
			class={twMerge(
				'mb-3 shrink-0 p-3 text-sm',
				connectionFailed ? 'notification-error' : 'notification-alert'
			)}
			role={connectionFailed ? 'alert' : 'status'}
		>
			<div class="flex flex-wrap items-center gap-2">
				<TriangleAlert class="size-4 shrink-0" aria-hidden="true" />
				<strong>{STATUS_LABEL[status]}</strong>
				{#if session.error}<span class="min-w-0 break-all">{session.error}</span>{/if}
				{#if onretry && (status === 'error' || status === 'unhealthy')}
					<button type="button" class="btn btn-secondary btn-sm ml-auto" onclick={onretry}>
						<RotateCw class="size-4" aria-hidden="true" />
						{m.mcps_retry()}
					</button>
				{/if}
			</div>
		</div>
	{/if}

	<div class="mb-2 flex shrink-0 flex-wrap items-center gap-2">
		<label class="relative min-w-48 flex-1">
			<Search
				class="text-muted-content absolute top-1/2 left-3 size-4 -translate-y-1/2"
				aria-hidden="true"
			/>
			<span class="sr-only">{m.mcps_tester_log_search_sr()}</span>
			<input
				class="text-input-filled h-10 pl-9"
				bind:value={query}
				type="search"
				placeholder={m.mcps_tester_log_search_placeholder()}
			/>
		</label>

		<span id="log-kind-label" class="sr-only">{m.mcps_tester_log_filter_by_kind()}</span>
		<Select
			id="log-kind"
			class="bg-base-200 dark:bg-base-200 dark:border-base-400 h-10 w-40 border border-transparent shadow-inner"
			options={KIND_OPTIONS}
			selected={kindFilter}
			ariaLabelledby="log-kind-label"
			onSelect={(option) => (kindFilter = option.id)}
		/>

		<button
			type="button"
			class="btn btn-secondary btn-sm"
			disabled={entries.length === 0}
			title={m.mcps_tester_log_copy_all_title()}
			onclick={copyLog}
		>
			{#if copied}
				<Check class="size-4" aria-hidden="true" /> {m.mcps_tester_log_copied()}
			{:else}
				<Copy class="size-4" aria-hidden="true" /> {m.mcps_tester_log_copy_all()}
			{/if}
		</button>

		<button
			type="button"
			class="btn btn-secondary btn-sm"
			disabled={entries.length === 0}
			onclick={clearLog}
		>
			<Trash2 class="size-4" aria-hidden="true" />
			{m.core_clear()}
		</button>
	</div>

	<p class="sr-only" aria-live="polite" aria-atomic="true">{announcement}</p>

	{#if dropped > 0}
		<p class="text-muted-content mb-2 shrink-0 text-xs">
			{dropped === 1
				? m.mcps_tester_log_dropped_one({ count: dropped })
				: m.mcps_tester_log_dropped_other({ count: dropped })}
		</p>
	{/if}

	{#if filtered.length === 0}
		<div class="bg-base-200 dark:bg-base-300 rounded-lg p-6 text-center text-sm text-muted-content">
			{#if !session}
				{m.mcps_tester_log_waiting()}
			{:else if entries.length > 0}
				<p>{m.mcps_tester_log_no_match()}</p>
				<button type="button" class="btn btn-secondary btn-sm mt-3" onclick={clearFilters}>
					{m.mcps_tester_log_clear_filters()}
				</button>
			{:else if status === 'setup-required'}
				{m.mcps_tester_log_empty_setup_required()}
			{:else if status === 'reauthentication-required'}
				{m.mcps_tester_log_empty_reauth_required()}
			{:else if status === 'unhealthy'}
				{m.mcps_tester_log_empty_unhealthy()}
			{:else}
				{m.mcps_tester_log_empty()}
			{/if}
		</div>
	{:else}
		<ul
			bind:this={listElement}
			onscroll={handleScroll}
			class="default-scrollbar-thin min-h-0 flex-1 space-y-1 overflow-y-auto pr-1 font-mono text-xs"
			aria-label={m.mcps_tester_log_traffic_log()}
		>
			{#each filtered as entry (entry.id)}
				{@const isOpen = expanded.has(entry.id)}
				<li class="border-base-300 dark:border-base-400 rounded-lg border">
					<button
						type="button"
						class="hover:bg-base-200 dark:hover:bg-base-300 flex w-full items-baseline gap-2 rounded-lg px-2 py-1.5 text-left"
						aria-expanded={isOpen}
						aria-controls={`log-entry-${entry.id}`}
						onclick={() => toggle(entry.id)}
					>
						<span class="text-muted-content shrink-0 tabular-nums">{clock(entry.at)}</span>
						<span class="shrink-0" aria-hidden="true">
							{entry.direction === 'outgoing' ? '→' : entry.direction === 'incoming' ? '←' : '•'}
						</span>
						<span class="sr-only">
							{entry.direction === 'outgoing'
								? m.mcps_tester_log_direction_sent()
								: entry.direction === 'incoming'
									? m.mcps_tester_log_direction_received()
									: m.mcps_tester_log_direction_local()}
						</span>
						<span class={twMerge('badge badge-xs shrink-0', KIND_BADGE[entry.kind])}>
							{KIND_LABEL[entry.kind]}
						</span>
						<span class="shrink-0 font-medium">{entry.method ?? '—'}</span>
						{#if entry.rpcId !== undefined}
							<span class="text-muted-content shrink-0">#{entry.rpcId}</span>
						{/if}
						<span class="text-muted-content min-w-0 flex-1 truncate">{summarize(entry)}</span>
						{#if entry.outcome === 'cancelled'}
							<span class="text-muted-content shrink-0">{m.mcps_tester_log_cancelled()}</span>
						{/if}
						{#if entry.durationMs !== undefined}
							<span class="text-muted-content shrink-0 tabular-nums"
								>{Math.round(entry.durationMs)} ms</span
							>
						{/if}
						<ChevronRight
							class={twMerge('size-3 shrink-0 transition-transform', isOpen && 'rotate-90')}
							aria-hidden="true"
						/>
					</button>

					{#if isOpen}
						{@const raw = rawFor(entry)}
						<div
							id={`log-entry-${entry.id}`}
							class="border-base-300 dark:border-base-400 space-y-2 border-t p-2"
						>
							{#if raw.length > MAX_INLINE_JSON && !forceRender.has(entry.id)}
								<div class="notification-alert p-3" role="status">
									<p>
										{m.mcps_tester_log_large_payload({ size: formatFileSize(raw.length) })}
									</p>
									<button
										type="button"
										class="btn btn-secondary btn-sm mt-2"
										onclick={() => forceRender.add(entry.id)}
									>
										{m.mcps_tester_log_render_anyway()}
									</button>
								</div>
							{:else}
								<CornerCopyButton text={raw} label={m.mcps_tester_log_copy_entry_json()}>
									<JsonPreview
										value={entry.message ?? entry}
										maxHeight="18rem"
										ariaLabel={m.mcps_tester_log_payload({
											name: entry.method ?? KIND_LABEL[entry.kind]
										})}
									/>
								</CornerCopyButton>
							{/if}
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</div>
