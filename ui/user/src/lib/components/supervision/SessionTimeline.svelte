<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		effectLabel,
		listSessionEvents,
		modeLabel,
		type ProvenanceEvent
	} from '$lib/services/provenance';
	import { userDeviceSettings } from '$lib/stores';
	import { formatLogTimestamp } from '$lib/time';
	import { resolve } from '$lib/url';
	import DecisionPill from './DecisionPill.svelte';
	import LabelBadges from './LabelBadges.svelte';
	import SurfaceBadge from './SurfaceBadge.svelte';
	import { ChevronDown, ChevronUp, Flag } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { SvelteMap, SvelteSet } from 'svelte/reactivity';
	import { twMerge } from 'tailwind-merge';

	// A session's events in causal order. Children are indented under their parent; a row expands
	// to show the decision's reason, the policies that matched and the lineage evidence. Every
	// field shown is content-free: the API never returns tool arguments.

	interface Props {
		sessionId: string;
		pageSize?: number;
	}

	let { sessionId, pageSize = 100 }: Props = $props();

	let events = $state<ProvenanceEvent[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let error = $state('');
	let expanded = new SvelteSet<string>();

	export async function load(reset = true) {
		loading = true;
		error = '';
		try {
			const offset = reset ? 0 : events.length;
			const resp = await listSessionEvents(
				sessionId,
				{ limit: pageSize, offset, ascending: true },
				{ dontLogErrors: true }
			);
			events = reset ? (resp.items ?? []) : [...events, ...(resp.items ?? [])];
			total = resp.total ?? events.length;
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			loading = false;
		}
	}

	onMount(() => load());

	let byID = $derived(new SvelteMap(events.map((e) => [e.id, e])));

	// Depth under the causal parent, bounded so deep chains stay readable.
	let depths = $derived.by(() => {
		const out: Record<string, number> = {};
		for (const e of events) {
			const parent = e.parentID ? out[e.parentID] : undefined;
			out[e.id] = parent === undefined ? 0 : Math.min(parent + 1, 4);
		}
		return out;
	});

	let hasMore = $derived(events.length < total);

	function hasDetails(e: ProvenanceEvent) {
		return !!(e.decision || e.lineage?.length || e.parentID);
	}

	function toggle(id: string) {
		if (expanded.has(id)) expanded.delete(id);
		else expanded.add(id);
	}

	function parentSummary(e: ProvenanceEvent) {
		if (!e.parentID) return '';
		const parent = byID.get(e.parentID);
		return parent?.summary || parent?.kind || e.parentID;
	}
</script>

<div class="flex flex-col gap-2">
	{#if error}
		<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
	{/if}

	{#if loading && events.length === 0}
		<div class="flex justify-center py-8"><Loading class="size-6" /></div>
	{:else if events.length === 0}
		<p class="text-muted-content py-6 text-center text-sm">{m.supervision_session_no_events()}</p>
	{:else}
		<ol class="flex flex-col" aria-label={m.supervision_session_timeline()}>
			{#each events as event (event.id)}
				{@const depth = depths[event.id] ?? 0}
				{@const isOpen = expanded.has(event.id)}
				{@const details = hasDetails(event)}
				<li
					class="border-base-300 dark:border-base-400 relative border-l py-2"
					style:padding-left={`${0.75 + depth * 1.25}rem`}
				>
					<span
						class={twMerge(
							'bg-base-100 dark:bg-base-200 absolute top-4 size-2.5 -translate-x-1/2 rounded-full border-2',
							event.decision && !event.decision.allow
								? 'border-error'
								: event.decision?.shadow && event.decision.effect !== 'allow'
									? 'border-warning'
									: 'border-primary'
						)}
						style:left="0"
					></span>
					<div class="flex flex-wrap items-center gap-2 text-sm">
						<span class="text-muted-content w-44 shrink-0 font-mono text-xs">
							{formatLogTimestamp(event.at, userDeviceSettings.timeFormat)}
						</span>
						<SurfaceBadge surface={event.surface} short />
						<code class="text-xs">{event.kind}</code>
						<span class="min-w-0 flex-1 truncate font-medium" title={event.summary}>
							{event.summary || ''}
						</span>
						<LabelBadges labels={event.labels} max={4} />
						{#if event.decision?.honeytoken}
							<span class="badge badge-sm badge-error gap-1">
								<Flag class="size-3" />
								{m.supervision_session_event_honeytoken()}
							</span>
						{/if}
						<DecisionPill decision={event.decision} />
						{#if details}
							<button
								type="button"
								class="btn btn-ghost btn-xs"
								aria-expanded={isOpen}
								aria-label={isOpen
									? m.supervision_session_hide_details()
									: m.supervision_session_show_details()}
								onclick={() => toggle(event.id)}
							>
								{#if isOpen}<ChevronUp class="size-4" />{:else}<ChevronDown class="size-4" />{/if}
							</button>
						{/if}
					</div>

					{#if isOpen}
						<div
							class="bg-base-200 dark:bg-base-300 mt-2 flex flex-col gap-2 rounded-md p-3 text-xs"
						>
							{#if event.parentID}
								<span class="text-muted-content">
									{m.supervision_session_event_parent({ summary: parentSummary(event) })}
								</span>
							{/if}

							{#if event.decision}
								<div class="flex flex-col gap-1">
									<span class="font-semibold">{m.supervision_session_event_reason()}</span>
									<p class="whitespace-pre-wrap">{event.decision.reason}</p>
								</div>

								{#if event.decision.matched?.length}
									<div class="flex flex-col gap-1">
										<span class="font-semibold">{m.supervision_session_event_policies()}</span>
										<ul class="flex flex-wrap gap-1">
											{#each event.decision.matched as match (match.policyID)}
												<li
													class="bg-base-100 dark:bg-base-200 flex items-center gap-1 rounded px-2 py-0.5"
												>
													<a href={resolve(`/policies/${match.policyID}`)} class="text-link">
														{match.policyName}
													</a>
													<span class="badge badge-xs badge-soft">{effectLabel(match.effect)}</span>
													<span class="badge badge-xs badge-ghost">
														{modeLabel(match.shadow ? 'shadow' : 'enforce')}
													</span>
												</li>
											{/each}
										</ul>
									</div>
								{/if}

								{#if event.decision.approvalID}
									<a
										href={resolve(`/approvals?approval=${event.decision.approvalID}`)}
										class="text-link w-fit"
									>
										{m.supervision_session_approval_link()}
									</a>
								{/if}
							{/if}

							{#if event.lineage?.length || event.decision?.lineage?.length}
								{@const hits = event.lineage?.length
									? event.lineage
									: (event.decision?.lineage ?? [])}
								<div class="flex flex-col gap-1">
									<span class="font-semibold">{m.supervision_session_event_lineage()}</span>
									<ul class="flex flex-col gap-0.5">
										{#each hits as hit, i (`${hit.originEventID}-${hit.label}-${i}`)}
											<li class="flex flex-wrap items-center gap-1">
												<span>
													{m.supervision_session_lineage_hit({
														label: hit.label,
														count: hit.count,
														origin: hit.originSummary || hit.originEventID
													})}
												</span>
												{#if hit.exact}
													<span class="badge badge-xs badge-error badge-soft">
														{m.supervision_session_lineage_exact()}
													</span>
												{/if}
											</li>
										{/each}
									</ul>
								</div>
							{/if}

							{#if event.bytes}
								<span class="text-muted-content">
									{m.supervision_session_event_bytes({ bytes: event.bytes })}
								</span>
							{/if}
						</div>
					{/if}
				</li>
			{/each}
		</ol>

		{#if hasMore}
			<button
				type="button"
				class="btn btn-secondary btn-sm self-center text-xs"
				disabled={loading}
				onclick={() => load(false)}
			>
				{#if loading}<Loading class="size-4" />{:else}{m.supervision_session_load_more()}{/if}
			</button>
		{/if}
	{/if}
</div>
