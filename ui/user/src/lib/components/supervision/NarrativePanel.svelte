<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { getSessionNarrative, type Narrative } from '$lib/services/provenance';
	import { RefreshCw, Sparkles } from '@lucide/svelte';
	import { onMount } from 'svelte';

	// The Supervisor's plain-language account of a session, or the engine's deterministic fallback.

	interface Props {
		sessionId: string;
	}

	let { sessionId }: Props = $props();

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

<section class="paper gap-3" aria-label={m.supervision_session_narrative()}>
	<div class="flex items-center justify-between gap-2">
		<h3 class="flex items-center gap-2 text-base font-semibold">
			<Sparkles class="text-primary size-4" />
			{m.supervision_session_narrative()}
		</h3>
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
		<p class="text-sm leading-relaxed">{narrative.summary}</p>

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

		{#if narrative.concerns?.length}
			<div class="flex flex-col gap-1">
				<h4 class="text-error text-xs font-semibold uppercase">
					{m.supervision_session_narrative_concerns()}
				</h4>
				<ul class="flex list-disc flex-col gap-1 pl-5 text-sm">
					{#each narrative.concerns as item, i (i)}
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
</section>
