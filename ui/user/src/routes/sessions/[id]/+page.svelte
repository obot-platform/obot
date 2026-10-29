<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import KillSwitchDialog from '$lib/components/supervision/KillSwitchDialog.svelte';
	import LabelBadges from '$lib/components/supervision/LabelBadges.svelte';
	import RiskPill from '$lib/components/supervision/RiskPill.svelte';
	import SessionTimeline from '$lib/components/supervision/SessionTimeline.svelte';
	import SupervisorCommentary from '$lib/components/supervision/SupervisorCommentary.svelte';
	import SurfaceBadge from '$lib/components/supervision/SurfaceBadge.svelte';
	import WarrantDialog from '$lib/components/supervision/WarrantDialog.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import {
		endSession,
		getSession,
		sessionDisplayName,
		SURFACES,
		surfaceLabel,
		type Session
	} from '$lib/services/provenance';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import { goto } from '$lib/url';
	import { Check, Minus, OctagonX, ScrollText, Square, TriangleAlert } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	// Session replay: the session's state and warrant, its event timeline with decisions and
	// lineage, and the Supervisor's commentary. While the session is open the page follows it
	// live.

	const LIVE_REFRESH_MS = 5_000;

	let sessionId = $derived(page.params.id ?? '');
	let session = $state<Session>();
	let loading = $state(true);
	let error = $state('');
	let notFound = $state(false);
	let killDialog = $state<ReturnType<typeof KillSwitchDialog>>();
	let warrantDialog = $state<ReturnType<typeof WarrantDialog>>();
	let timeline = $state<ReturnType<typeof SessionTimeline>>();
	let commentary = $state<ReturnType<typeof SupervisorCommentary>>();
	let confirmEnd = $state(false);
	let ending = $state(false);

	let readonly = $derived(profile.current.isAdminReadonly?.());
	let title = $derived(session ? sessionDisplayName(session) : m.supervision_session_title());
	let reported = $derived(new Set(session?.state.surfaces ?? []));

	async function load(quiet = false) {
		if (!quiet) loading = true;
		error = '';
		notFound = false;
		try {
			session = await getSession(sessionId, { dontLogErrors: true });
		} catch (e) {
			const parsed = parseErrorContent(e);
			if (parsed.status === 404) notFound = true;
			else error = parsed.message;
		} finally {
			loading = false;
		}
	}

	onMount(() => load());

	let live = $derived(!!session && !session.endedAt && !session.state.killed);

	// Follow an open session: refresh its state and timeline on an interval. setInterval, not
	// setTimeout, so the cadence is not shortened by the test harness.
	$effect(() => {
		if (!live) return;
		const timer = setInterval(() => {
			load(true);
			timeline?.refresh();
		}, LIVE_REFRESH_MS);
		return () => clearInterval(timer);
	});

	async function refreshAll() {
		await Promise.all([load(), timeline?.load(), commentary?.load()]);
	}

	async function end() {
		ending = true;
		try {
			await endSession(sessionId, { dontLogErrors: true });
			success.add(m.supervision_session_ended_toast());
			confirmEnd = false;
			await refreshAll();
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			ending = false;
		}
	}

	let counters = $derived(
		session
			? [
					{ label: m.supervision_col_events(), value: session.state.events },
					{ label: m.supervision_session_tool_calls(), value: session.state.toolCalls },
					{ label: m.supervision_session_denied(), value: session.state.denied },
					{ label: m.supervision_session_shadowed(), value: session.state.shadowed },
					{ label: m.supervision_session_approvals(), value: session.state.approvals },
					{ label: m.supervision_session_external_out(), value: session.state.externalOut }
				]
			: []
	);
</script>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>

<Layout
	{title}
	showBackButton
	onBackButtonClick={() => goto('/sessions')}
	rightNavActions={navActions}
>
	<div class="flex flex-col gap-4 pb-8">
		{#if error}
			<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
		{/if}

		{#if loading && !session}
			<Skeleton type="card" class="min-h-40 w-full" />
			<Skeleton type="list" count={6} />
		{:else if notFound}
			<p class="text-muted-content py-12 text-center text-sm">
				{m.supervision_session_not_found()}
			</p>
		{:else if session}
			{#if session.state.killed}
				<div class="notification-error flex items-center gap-2 rounded-md p-3 text-sm" role="alert">
					<OctagonX class="size-4 shrink-0" />
					{m.supervision_session_killed_banner()}
				</div>
			{/if}

			<!-- Header -->
			<section class="paper gap-3">
				<div class="flex flex-wrap items-start justify-between gap-3">
					<div class="flex min-w-0 flex-col gap-1">
						<div class="flex flex-wrap items-center gap-2">
							<RiskPill risk={session.state.risk} labelled />
							{#if session.endedAt}
								<span class="badge badge-sm badge-ghost">{m.supervision_sessions_ended()}</span>
							{:else}
								<span class="badge badge-sm badge-soft badge-success"
									>{m.supervision_sessions_open()}</span
								>
							{/if}
							{#if session.state.quarantined}
								<span class="badge badge-sm badge-soft badge-warning"
									>{m.supervision_session_quarantined()}</span
								>
							{/if}
							{#if session.state.downshifted}
								<span class="badge badge-sm badge-soft badge-warning"
									>{m.supervision_session_downshifted()}</span
								>
							{/if}
						</div>
						<dl class="text-muted-content flex flex-wrap gap-x-4 gap-y-1 text-xs">
							<div class="flex gap-1">
								<dt>{m.core_col_user()}:</dt>
								<dd class="text-base-content">
									{session.email || session.userID || m.core_unknown_user()}
								</dd>
							</div>
							<div class="flex gap-1">
								<dt>{m.supervision_col_agent()}:</dt>
								<dd class="text-base-content">
									{session.agent || session.actor?.agent || m.core_unknown()}
									{#if session.actor?.agentVersion}
										<span class="text-muted-content">{session.actor.agentVersion}</span>
									{/if}
								</dd>
							</div>
							<div class="flex gap-1">
								<dt>{m.supervision_col_started()}:</dt>
								<dd class="text-base-content" title={formatTimeAgo(session.startedAt).fullDate}>
									{formatTimeAgo(session.startedAt).relativeTime}
								</dd>
							</div>
							<div class="flex gap-1">
								<dt>{m.supervision_col_last_activity()}:</dt>
								<dd
									class="text-base-content"
									title={formatTimeAgo(session.lastActivityAt).fullDate}
								>
									{formatTimeAgo(session.lastActivityAt).relativeTime}
								</dd>
							</div>
							<div class="flex gap-1">
								<dt>ID:</dt>
								<dd><code class="text-base-content">{session.id}</code></dd>
							</div>
						</dl>
					</div>
					<div class="flex flex-wrap gap-4">
						{#each counters as c (c.label)}
							<div class="flex flex-col items-end">
								<span class="text-lg font-semibold tabular-nums">{c.value}</span>
								<span class="text-muted-content text-xs">{c.label}</span>
							</div>
						{/each}
					</div>
				</div>

				<div class="flex flex-col gap-1">
					<span class="text-muted-content text-xs uppercase">{m.supervision_labels()}</span>
					{#if session.state.labels?.length}
						<LabelBadges labels={session.state.labels} origins={session.state.origins} />
					{:else}
						<span class="text-muted-content text-sm">{m.supervision_none()}</span>
					{/if}
				</div>

				<div class="flex flex-col gap-1">
					<span class="text-muted-content text-xs uppercase">{m.supervision_session_warrant()}</span
					>
					{#if session.state.warrant}
						{@const w = session.state.warrant}
						<div class="bg-base-200 dark:bg-base-300 flex flex-col gap-1 rounded-md p-3 text-sm">
							<div class="flex flex-wrap items-center gap-2">
								<ScrollText class="size-4 shrink-0" />
								<span class="font-medium">{w.task}</span>
								{#if w.drifted}
									<span class="badge badge-sm badge-soft badge-warning gap-1">
										<TriangleAlert class="size-3" />
										{m.supervision_session_warrant_drifted()}
									</span>
								{/if}
								<span class="text-muted-content text-xs">
									{m.supervision_session_warrant_expires({
										time: formatTimeUntil(w.expiresAt).relativeTime
									})}
								</span>
							</div>
							<div class="flex flex-wrap gap-1">
								{#each w.scopes as scope (scope)}
									<code class="bg-base-100 dark:bg-base-200 rounded px-1.5 py-0.5 text-xs"
										>{scope}</code
									>
								{/each}
							</div>
							{#if w.drifted && w.driftSummary}
								<span class="text-warning text-xs">
									{m.supervision_session_warrant_drift_summary({ summary: w.driftSummary })}
								</span>
							{/if}
						</div>
					{:else}
						<span class="text-muted-content text-sm">{m.supervision_session_warrant_none()}</span>
					{/if}
				</div>
			</section>

			<div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
				<section class="paper gap-3 lg:col-span-2" aria-label={m.supervision_session_timeline()}>
					<div class="flex items-baseline justify-between gap-2">
						<h2 class="text-base font-semibold">{m.supervision_session_timeline()}</h2>
						<span class="text-muted-content text-xs">
							{m.supervision_session_events_count({ count: session.state.events })}
						</span>
					</div>
					<SessionTimeline bind:this={timeline} {sessionId} />
				</section>

				<div class="flex flex-col gap-4">
					<SupervisorCommentary bind:this={commentary} {sessionId} {live} />

					<section class="paper gap-2" aria-label={m.supervision_session_coverage()}>
						<h3 class="text-base font-semibold">{m.supervision_session_coverage()}</h3>
						<ul class="flex flex-col gap-1 text-sm">
							{#each SURFACES as surface (surface)}
								{@const seen = reported.has(surface)}
								<li class={twMerge('flex items-center gap-2', !seen && 'opacity-60')}>
									{#if seen}
										<Check class="text-success size-4" />
									{:else}
										<Minus class="text-muted-content size-4" />
									{/if}
									<SurfaceBadge {surface} short />
									<span>{surfaceLabel(surface)}</span>
									<span class="text-muted-content ml-auto text-xs">
										{seen
											? m.supervision_session_coverage_reported()
											: m.supervision_session_coverage_missing()}
									</span>
								</li>
							{/each}
						</ul>
					</section>
				</div>
			</div>
		{/if}
	</div>
</Layout>

{#snippet navActions()}
	{#if session && !readonly}
		<div class="flex items-center gap-2">
			<button
				type="button"
				class="btn btn-secondary rounded-4xl text-sm"
				onclick={() => warrantDialog?.open(session?.state.warrant)}
			>
				<ScrollText class="size-4" />
				{m.supervision_session_issue_warrant()}
			</button>
			{#if !session.endedAt}
				<button
					type="button"
					class="btn btn-secondary rounded-4xl text-sm"
					onclick={() => (confirmEnd = true)}
				>
					<Square class="size-4" />
					{m.supervision_session_end()}
				</button>
			{/if}
			{#if !session.state.killed}
				<button
					type="button"
					class="btn btn-error rounded-4xl text-sm"
					onclick={() => killDialog?.open({ scope: 'session', target: sessionId })}
				>
					<OctagonX class="size-4" />
					{m.supervision_session_kill()}
				</button>
			{/if}
		</div>
	{/if}
{/snippet}

<KillSwitchDialog bind:this={killDialog} onCreated={refreshAll} />
<WarrantDialog bind:this={warrantDialog} {sessionId} onIssued={refreshAll} />
<Confirm
	show={confirmEnd}
	type="info"
	msg={m.supervision_session_end_confirm()}
	submitText={m.supervision_session_end()}
	loading={ending}
	onsuccess={end}
	oncancel={() => (confirmEnd = false)}
/>
