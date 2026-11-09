<script lang="ts">
	import LabelBadges from '$lib/components/supervision/LabelBadges.svelte';
	import RiskPill from '$lib/components/supervision/RiskPill.svelte';
	import SurfaceBadge from '$lib/components/supervision/SurfaceBadge.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { getSession, sessionDisplayName, type Session } from '$lib/services/provenance';
	import { formatTimeAgo } from '$lib/time';
	import { resolve } from '$lib/url';
	import WidgetFrame from './WidgetFrame.svelte';
	import { onMount } from 'svelte';

	// A short list of sessions, in the order Obot AI chose, styled like the Sessions page.

	let { sessionIds }: { sessionIds: string[] } = $props();

	let sessions = $state<Session[]>([]);
	let error = $state('');
	let loading = $state(true);

	onMount(async () => {
		const results = await Promise.allSettled(
			sessionIds.map((id) => getSession(id, { dontLogErrors: true }))
		);
		sessions = results.flatMap((r) => (r.status === 'fulfilled' ? [r.value] : []));
		const failed = results.find((r) => r.status === 'rejected');
		if (!sessions.length && failed?.status === 'rejected') {
			const parsed = parseErrorContent(failed.reason);
			error = parsed.status === 404 ? m.ai_widget_not_found() : parsed.message;
		}
		loading = false;
	});
</script>

<WidgetFrame {loading} {error}>
	<p class="text-muted-content text-xs">{m.ai_widget_sessions({ count: sessions.length })}</p>
	<ul class="divide-base-300 dark:divide-base-400 flex flex-col divide-y">
		{#each sessions as session (session.id)}
			<li class="flex flex-wrap items-center gap-x-3 gap-y-1 py-2">
				<RiskPill risk={session.state.risk} />
				<div class="flex min-w-0 flex-1 flex-col">
					<a href={resolve(`/sessions/${session.id}`)} class="truncate font-medium hover:underline">
						{sessionDisplayName(session)}
					</a>
					<span class="text-muted-content truncate text-xs">
						{[session.email, session.state.lastSummary].filter(Boolean).join(' · ')}
					</span>
				</div>
				<div class="flex flex-wrap items-center gap-1">
					{#each session.state.surfaces ?? [] as surface (surface)}
						<SurfaceBadge {surface} short />
					{/each}
					<LabelBadges labels={session.state.labels} max={3} />
				</div>
				<span
					class="text-muted-content text-xs"
					title={formatTimeAgo(session.lastActivityAt).fullDate}
				>
					{formatTimeAgo(session.lastActivityAt).relativeTime}
				</span>
			</li>
		{/each}
	</ul>
</WidgetFrame>
