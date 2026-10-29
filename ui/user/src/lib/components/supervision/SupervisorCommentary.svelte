<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { getSessionNarrative, type Narrative } from '$lib/services/provenance';
	import SupervisorAsk from './SupervisorAsk.svelte';
	import SupervisorAvatar from './SupervisorAvatar.svelte';
	import { RefreshCw } from '@lucide/svelte';
	import { onMount } from 'svelte';

	// The Supervisor's commentary on one session: its account of what happened, what concerns
	// it, and a place to ask it about this session.

	interface Props {
		sessionId: string;
		live?: boolean;
	}

	let { sessionId, live = false }: Props = $props();

	let narrative = $state<Narrative>();
	let loading = $state(true);
	let error = $state('');

	export async function load() {
		loading = true;
		error = '';
		try {
			narrative = await getSessionNarrative(sessionId, { dontLogErrors: true });
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<section class="paper border-primary/30 gap-3 border" aria-label={m.supervision_commentary_title()}>
	<div class="flex items-start justify-between gap-2">
		<div class="flex items-center gap-2">
			<SupervisorAvatar pulse={live} />
			<div class="flex flex-col">
				<h3 class="text-base font-semibold">{m.supervision_commentary_title()}</h3>
				<span class="text-muted-content text-xs">
					{live ? m.supervision_commentary_live() : m.supervision_commentary_subtitle()}
				</span>
			</div>
		</div>
		<button
			type="button"
			class="btn btn-ghost btn-sm"
			aria-label={m.supervision_refresh()}
			disabled={loading}
			onclick={load}
		>
			<RefreshCw class="size-4 {loading ? 'animate-spin' : ''}" />
		</button>
	</div>

	{#if loading && !narrative}
		<div class="flex justify-center py-6"><Loading class="size-6" /></div>
	{:else if error}
		<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
	{:else if !narrative?.summary}
		<p class="text-muted-content text-sm">{m.supervision_session_narrative_none()}</p>
	{:else}
		<p class="bg-primary/5 rounded-2xl rounded-tl-sm px-3 py-2 text-sm leading-relaxed">
			{narrative.summary}
		</p>

		{#if narrative.concerns?.length}
			<div class="flex flex-col gap-1">
				<h4 class="text-error text-xs font-semibold uppercase">
					{m.supervision_commentary_concerns()}
				</h4>
				<ul class="flex list-disc flex-col gap-1 pl-5 text-sm">
					{#each narrative.concerns as item, i (i)}
						<li>{item}</li>
					{/each}
				</ul>
			</div>
		{/if}

		{#if narrative.timeline?.length}
			<div class="flex flex-col gap-1">
				<h4 class="text-muted-content text-xs font-semibold uppercase">
					{m.supervision_session_narrative_timeline()}
				</h4>
				<ul class="flex list-disc flex-col gap-1 pl-5 text-sm">
					{#each narrative.timeline as item, i (i)}
						<li>{item}</li>
					{/each}
				</ul>
			</div>
		{/if}

		{#if narrative.model}
			<span class="text-muted-content text-xs">
				{m.supervision_session_narrative_model({ model: narrative.model })}
			</span>
		{/if}
	{/if}

	<div class="border-base-300 dark:border-base-400 border-t pt-3">
		<SupervisorAsk
			{sessionId}
			embedded
			intro={m.supervision_ask_session_intro()}
			suggestions={[
				m.supervision_ask_suggest_session_safe(),
				m.supervision_ask_suggest_session_data(),
				m.supervision_ask_suggest_last_hold()
			]}
		/>
	</div>
</section>
