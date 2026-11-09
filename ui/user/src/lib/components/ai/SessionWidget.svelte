<script lang="ts">
	import LabelBadges from '$lib/components/supervision/LabelBadges.svelte';
	import RiskPill from '$lib/components/supervision/RiskPill.svelte';
	import SessionTimeline from '$lib/components/supervision/SessionTimeline.svelte';
	import SurfaceBadge from '$lib/components/supervision/SurfaceBadge.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { getSession, sessionDisplayName, type Session } from '$lib/services/provenance';
	import { formatTimeAgo } from '$lib/time';
	import { resolve } from '$lib/url';
	import WidgetFrame from './WidgetFrame.svelte';
	import { ExternalLink } from '@lucide/svelte';
	import { onMount } from 'svelte';

	// A session as Obot AI shows it in an answer: its header and its event stream, the same
	// timeline as the session page. It follows an open session live.

	const LIVE_REFRESH_MS = 5_000;

	let { sessionId }: { sessionId: string } = $props();

	let session = $state<Session>();
	let error = $state('');
	let loading = $state(true);
	let timeline = $state<ReturnType<typeof SessionTimeline>>();

	async function load() {
		try {
			session = await getSession(sessionId, { dontLogErrors: true });
			error = '';
		} catch (e) {
			const parsed = parseErrorContent(e);
			error = parsed.status === 404 ? m.ai_widget_not_found() : parsed.message;
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		load();
	});

	let live = $derived(!!session && !session.endedAt && !session.state.killed);

	$effect(() => {
		if (!live) return;
		const timer = setInterval(() => {
			load();
			timeline?.refresh();
		}, LIVE_REFRESH_MS);
		return () => clearInterval(timer);
	});
</script>

<WidgetFrame {loading} {error}>
	{#if session}
		<header class="flex flex-wrap items-start justify-between gap-2">
			<div class="flex min-w-0 flex-col gap-1">
				<div class="flex flex-wrap items-center gap-2">
					<RiskPill risk={session.state.risk} labelled />
					<a href={resolve(`/sessions/${session.id}`)} class="truncate font-medium hover:underline">
						{sessionDisplayName(session)}
					</a>
					{#if live}
						<span
							class="flex items-center gap-1 rounded-full bg-success/15 text-success px-2 py-0.5 text-xs"
						>
							<span class="size-1.5 animate-pulse rounded-full bg-success"></span>
							{m.ai_widget_live()}
						</span>
					{/if}
				</div>
				<div class="text-muted-content flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
					{#if session.email}<span>{session.email}</span>{/if}
					<span title={formatTimeAgo(session.lastActivityAt).fullDate}>
						{formatTimeAgo(session.lastActivityAt).relativeTime}
					</span>
					<span>{m.supervision_col_events()}: {session.state.events}</span>
					<span>{m.supervision_session_tool_calls()}: {session.state.toolCalls}</span>
					{#if session.state.denied}
						<span class="text-red-600"
							>{m.supervision_session_denied()}: {session.state.denied}</span
						>
					{/if}
				</div>
				<div class="flex flex-wrap items-center gap-1">
					{#each session.state.surfaces ?? [] as surface (surface)}
						<SurfaceBadge {surface} short />
					{/each}
					<LabelBadges labels={session.state.labels} origins={session.state.origins} max={6} />
				</div>
			</div>
			<a
				href={resolve(`/sessions/${session.id}`)}
				class="btn btn-secondary flex items-center gap-1 rounded-4xl px-3 py-1 text-xs"
			>
				{m.ai_widget_open_session()}
				<ExternalLink class="size-3" />
			</a>
		</header>
		<div class="max-h-96 overflow-y-auto">
			<SessionTimeline bind:this={timeline} {sessionId} pageSize={30} />
		</div>
	{/if}
</WidgetFrame>
