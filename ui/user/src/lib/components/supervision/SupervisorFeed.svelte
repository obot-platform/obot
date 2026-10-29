<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { isIntervention, listEvents, type ProvenanceEvent } from '$lib/services/provenance';
	import DecisionVoice from './DecisionVoice.svelte';
	import { Activity } from '@lucide/svelte';
	import { fly } from 'svelte/transition';

	// The live activity feed: every decision across every surface, newest first, told by the
	// Supervisor. It polls while the page is open so the reader watches decisions as they land.

	interface Props {
		limit?: number;
		intervalMs?: number;
		onEvents?: (events: ProvenanceEvent[]) => void;
	}

	let { limit = 40, intervalMs = 5_000, onEvents }: Props = $props();

	let events = $state<ProvenanceEvent[]>([]);
	let loading = $state(true);
	let error = $state('');
	let onlyInterventions = $state(false);

	export async function load() {
		try {
			const resp = await listEvents({ limit }, { dontLogErrors: true });
			events = resp.items ?? [];
			error = '';
			onEvents?.(events);
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			loading = false;
		}
	}

	// setInterval, not setTimeout, so the cadence is not shortened by the test harness.
	$effect(() => {
		load();
		const timer = setInterval(load, intervalMs);
		return () => clearInterval(timer);
	});

	let shown = $derived(onlyInterventions ? events.filter(isIntervention) : events);
</script>

<section class="paper gap-3" aria-label={m.supervision_feed_title()}>
	<div class="flex flex-wrap items-center justify-between gap-2">
		<h2 class="flex items-center gap-2 text-base font-semibold">
			<Activity class="text-primary size-4" />
			{m.supervision_feed_title()}
			<span class="badge badge-xs badge-soft badge-success gap-1">
				<span class="bg-success size-1.5 animate-pulse rounded-full"></span>
				{m.supervision_feed_live()}
			</span>
		</h2>
		<label class="flex items-center gap-2 text-xs">
			<input type="checkbox" class="checkbox checkbox-xs" bind:checked={onlyInterventions} />
			{m.supervision_feed_interventions()}
		</label>
	</div>

	{#if error && events.length === 0}
		<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
	{:else if loading}
		<div class="flex justify-center py-6"><Loading class="size-6" /></div>
	{:else if shown.length === 0}
		<p class="text-muted-content py-4 text-center text-sm">{m.supervision_feed_empty()}</p>
	{:else}
		<ol class="flex max-h-[36rem] flex-col gap-1 overflow-y-auto">
			{#each shown as event (event.id)}
				<li in:fly={{ y: -8, duration: 250 }}>
					<DecisionVoice {event} />
				</li>
			{/each}
		</ol>
	{/if}
</section>
