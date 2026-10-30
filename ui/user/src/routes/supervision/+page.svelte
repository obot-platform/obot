<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import ActiveKills from '$lib/components/supervision/ActiveKills.svelte';
	import ApprovalCard from '$lib/components/supervision/ApprovalCard.svelte';
	import KillSwitchDialog from '$lib/components/supervision/KillSwitchDialog.svelte';
	import LabelBadges from '$lib/components/supervision/LabelBadges.svelte';
	import RedTeamScorecard from '$lib/components/supervision/RedTeamScorecard.svelte';
	import RiskPill from '$lib/components/supervision/RiskPill.svelte';
	import SupervisorAsk from '$lib/components/supervision/SupervisorAsk.svelte';
	import SupervisorAvatar from '$lib/components/supervision/SupervisorAvatar.svelte';
	import SupervisorBriefing from '$lib/components/supervision/SupervisorBriefing.svelte';
	import SupervisorFeed from '$lib/components/supervision/SupervisorFeed.svelte';
	import SupervisorInsights from '$lib/components/supervision/SupervisorInsights.svelte';
	import SupervisorPresence from '$lib/components/supervision/SupervisorPresence.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		getOverview,
		getSupervisorInsights,
		getSupervisorStatus,
		listApprovals,
		listKills,
		runRedTeam,
		sessionDisplayName,
		SURFACES,
		surfaceLabel,
		type Approval,
		type Kill,
		type Overview,
		type ProvenanceEvent,
		type RedTeamResult,
		type SupervisorInsights as Insights,
		type SupervisorStatus
	} from '$lib/services/provenance';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { resolve } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { CircleHelp, Inbox, OctagonX, Swords } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	// The Supervisor's home. The reader looks over the Supervisor's shoulder: whether it is on and
	// what it watches, its briefing, the held actions waiting on a person (its recommendation
	// first), what it noticed, every decision as it lands, and a place to ask it questions.
	// The kill switch and red team sit below as controls.

	const LIVE_REFRESH_MS = 5_000;
	const SLOW_REFRESH_MS = 30_000;
	const ATTENTION_LIMIT = 3;

	let hours = $state(24);
	let overview = $state<Overview>();
	let status = $state<SupervisorStatus>();
	let statusLive = $state(true);
	let insights = $state<Insights>();
	let insightsLoading = $state(true);
	let insightsUnavailable = $state(false);
	let refreshingBriefing = $state(false);
	let attention = $state<Approval[]>([]);
	let attentionTotal = $state(0);
	let attentionLoading = $state(true);
	let latestEventAt = $state<string>();
	let kills = $state<Kill[]>([]);
	let loading = $state(true);
	let error = $state('');
	let killDialog = $state<ReturnType<typeof KillSwitchDialog>>();
	let redTeam = $state<RedTeamResult>();
	let runningRedTeam = $state(false);
	let redTeamError = $state('');

	let readonly = $derived(profile.current.isAdminReadonly?.());

	async function loadOverview() {
		try {
			overview = await getOverview(hours, { dontLogErrors: true });
			error = '';
		} catch (e) {
			error = parseErrorContent(e).message;
		}
	}

	async function loadStatus() {
		try {
			status = await getSupervisorStatus({ dontLogErrors: true });
			statusLive = true;
		} catch {
			// Older servers have no status endpoint: describe the Supervisor from the overview.
			statusLive = false;
		}
	}

	async function loadInsights(refresh = false) {
		try {
			insights = await getSupervisorInsights(refresh, { dontLogErrors: true });
			insightsUnavailable = false;
		} catch {
			insightsUnavailable = true;
		} finally {
			insightsLoading = false;
		}
	}

	async function loadAttention() {
		try {
			const resp = await listApprovals(
				{ status: 'pending', limit: ATTENTION_LIMIT },
				{ dontLogErrors: true }
			);
			attention = resp.items ?? [];
			attentionTotal = Math.max(resp.total ?? 0, attention.length);
		} catch {
			// The attention queue is best effort; the approvals page shows errors.
		} finally {
			attentionLoading = false;
		}
	}

	async function load() {
		loading = true;
		await Promise.all([
			loadOverview(),
			loadStatus(),
			loadInsights(),
			loadAttention(),
			listKills({ dontLogErrors: true })
				.then((k) => (kills = k))
				.catch(() => undefined)
		]);
		loading = false;
	}

	onMount(() => {
		load();
		// setInterval, not setTimeout, so the cadence is not shortened by the test harness.
		const fast = setInterval(() => {
			loadStatus();
			loadAttention();
		}, LIVE_REFRESH_MS);
		const slow = setInterval(() => {
			loadInsights();
			loadOverview();
		}, SLOW_REFRESH_MS);
		return () => {
			clearInterval(fast);
			clearInterval(slow);
		};
	});

	async function setHours(h: number) {
		hours = h;
		await loadOverview();
	}

	async function refreshBriefing() {
		refreshingBriefing = true;
		await loadInsights(true);
		refreshingBriefing = false;
	}

	async function runRed() {
		runningRedTeam = true;
		redTeamError = '';
		try {
			redTeam = await runRedTeam({ dontLogErrors: true });
		} catch (e) {
			redTeamError = parseErrorContent(e).message;
		} finally {
			runningRedTeam = false;
		}
	}

	function onFeed(events: ProvenanceEvent[]) {
		latestEventAt = events[0]?.at;
	}

	function onResolved(resolved: Approval) {
		attention = attention.filter((a) => a.id !== resolved.id);
		attentionTotal = Math.max(0, attentionTotal - 1);
		loadAttention();
		loadStatus();
	}

	// What the presence header shows: the live status, or a description built from the overview
	// and the feed when the server has no status endpoint.
	let presence = $derived.by<SupervisorStatus | undefined>(() => {
		if (statusLive && status) return status;
		if (!overview) return undefined;
		const o = overview;
		return {
			enabled: o.supervisor.enabled,
			configured: true,
			model: o.supervisor.model,
			watching: {
				sessions: o.coverage.sessions,
				openSessions: 0,
				surfaces: SURFACES.filter((s) => (o.coverage.surfaces[s] ?? 0) > 0)
			},
			lastDecisionAt: latestEventAt ?? null,
			lastJudgementAt: null,
			today: {
				decided: o.decisions.allowed + o.decisions.denied,
				judged: o.byEffect.judge ?? 0,
				held: o.byEffect.require_approval ?? 0,
				denied: o.decisions.denied,
				allowed: o.decisions.allowed,
				shadowed: o.decisions.shadowed,
				triaged: 0
			},
			pendingApprovals: o.decisions.approvals_pending
		};
	});

	let riskiestRows = $derived(
		(overview?.riskiest ?? []).map((s) => ({
			id: s.id,
			name: sessionDisplayName(s),
			user: s.email || s.userID || '',
			agent: s.agent || s.actor?.agent || '',
			labels: s.state.labels ?? [],
			risk: s.state.risk,
			lastActivityAt: s.lastActivityAt
		}))
	);
</script>

<svelte:head>
	<title>Obot | {m.supervision_overview_title()}</title>
</svelte:head>

<Layout title={m.supervision_overview_title()} rightNavActions={navActions}>
	<div class="flex flex-col gap-4 pb-8">
		{#if error}
			<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
		{/if}

		<SupervisorPresence status={presence} loading={loading && !presence} onChanged={load} />

		{#if loading && !overview}
			<Skeleton type="card" class="min-h-16 w-full" />
		{:else if overview}
			<!-- Coverage strip -->
			<section
				class="paper flex-row flex-wrap items-center gap-x-6 gap-y-2 py-3"
				aria-label={m.supervision_overview_coverage()}
			>
				<div class="flex flex-col">
					<h2 class="text-sm font-semibold">{m.supervision_overview_coverage()}</h2>
					<span class="text-muted-content text-xs">
						{m.supervision_hours_window({ hours })}
					</span>
				</div>
				<div class="flex flex-col">
					<span class="text-muted-content text-xs">{m.supervision_overview_sessions()}</span>
					<span class="text-lg font-semibold tabular-nums">{overview.coverage.sessions}</span>
				</div>
				{#each SURFACES as surface (surface)}
					{@const count = overview.coverage.surfaces[surface] ?? 0}
					<div class={twMerge('flex flex-col', count === 0 && 'opacity-50')}>
						<span class="text-muted-content truncate text-xs">{surfaceLabel(surface)}</span>
						<span class="text-lg font-semibold tabular-nums">{count}</span>
					</div>
				{/each}
				<div
					class={twMerge(
						'flex flex-col rounded-md px-2',
						overview.coverage.blind > 0 && 'bg-warning/15'
					)}
					title={m.supervision_overview_blind_sessions_help()}
				>
					<span class="text-muted-content flex items-center gap-1 text-xs">
						{m.supervision_overview_blind_sessions()}
						<CircleHelp class="size-3" />
					</span>
					<span class="text-lg font-semibold tabular-nums">{overview.coverage.blind}</span>
				</div>
			</section>
		{/if}

		<div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
			<div class="flex min-w-0 flex-col gap-4 lg:col-span-2">
				<SupervisorBriefing
					{insights}
					loading={insightsLoading}
					refreshing={refreshingBriefing}
					unavailable={insightsUnavailable}
					onRefresh={refreshBriefing}
				/>

				<!-- Needs your attention -->
				<section class="paper gap-3" aria-label={m.supervision_attention_title()}>
					<div class="flex flex-wrap items-baseline justify-between gap-2">
						<h2 class="flex items-center gap-2 text-base font-semibold">
							<Inbox class="text-warning size-4" />
							{m.supervision_attention_title()}
							{#if attentionTotal > 0}
								<span class="badge badge-sm badge-warning">{attentionTotal}</span>
							{/if}
						</h2>
						<a href={resolve('/approvals')} class="text-link text-sm">
							{m.supervision_attention_view_all()}
						</a>
					</div>
					{#if attentionLoading && attention.length === 0}
						<Skeleton type="card" class="min-h-24 w-full" />
					{:else if attention.length === 0}
						<div class="text-muted-content flex items-center gap-2 text-sm">
							<SupervisorAvatar size="sm" active={false} />
							{m.supervision_attention_empty()}
						</div>
					{:else}
						<div class="flex flex-col gap-3">
							{#each attention as approval (approval.id)}
								<ApprovalCard {approval} {onResolved} compact />
							{/each}
						</div>
						{#if attentionTotal > attention.length}
							<a href={resolve('/approvals')} class="text-link self-center text-sm">
								{m.supervision_attention_more({ count: attentionTotal - attention.length })}
							</a>
						{/if}
					{/if}
				</section>

				<SupervisorFeed onEvents={onFeed} />
			</div>

			<div class="flex min-w-0 flex-col gap-4">
				<SupervisorInsights
					items={insights?.items ?? []}
					loading={insightsLoading}
					error={insightsUnavailable ? m.supervision_insights_unavailable() : ''}
					generatedAt={insights?.generatedAt}
					onActed={() => Promise.all([loadInsights(true), loadStatus(), loadOverview()])}
				/>
				<SupervisorAsk
					intro={m.supervision_ask_intro()}
					suggestions={[
						m.supervision_ask_suggest_riskiest(),
						m.supervision_ask_suggest_last_hold(),
						m.supervision_ask_suggest_next_policy()
					]}
				/>
			</div>
		</div>

		<!-- Riskiest sessions -->
		{#if overview}
			<section class="paper gap-3" aria-label={m.supervision_overview_riskiest()}>
				<div class="flex items-baseline justify-between gap-2">
					<h2 class="text-base font-semibold">{m.supervision_overview_riskiest()}</h2>
					<a href={resolve('/sessions')} class="text-link text-sm"
						>{m.supervision_view_all_sessions()}</a
					>
				</div>
				{#if riskiestRows.length === 0}
					<p class="text-muted-content text-sm">{m.supervision_overview_no_sessions()}</p>
				{:else}
					<Table
						data={riskiestRows}
						fields={['risk', 'name', 'labels', 'agent', 'user', 'lastActivityAt']}
						headers={[
							{ property: 'risk', title: m.supervision_risk() },
							{ property: 'name', title: m.supervision_col_session() },
							{ property: 'labels', title: m.supervision_labels() },
							{ property: 'agent', title: m.supervision_col_agent() },
							{ property: 'user', title: m.core_col_user() },
							{ property: 'lastActivityAt', title: m.supervision_col_last_activity() }
						]}
						onClickRow={(row, isCtrl) => openUrl(`/sessions/${row.id}`, isCtrl)}
					>
						{#snippet onRenderColumn(property, row)}
							{#if property === 'risk'}
								<RiskPill risk={row.risk} />
							{:else if property === 'labels'}
								<LabelBadges labels={row.labels} max={3} />
							{:else if property === 'lastActivityAt'}
								<span class="text-xs" title={formatTimeAgo(row.lastActivityAt).fullDate}>
									{formatTimeAgo(row.lastActivityAt).relativeTime}
								</span>
							{:else if property === 'name'}
								<a href={resolve(`/sessions/${row.id}`)} class="font-medium hover:underline">
									{row.name}
								</a>
							{:else}
								{row[property as keyof typeof row]}
							{/if}
						{/snippet}
					</Table>
				{/if}
			</section>
		{/if}

		<!-- Controls: policies, the kill switch and the red team -->
		<h2 class="text-muted-content mt-2 text-xs font-semibold uppercase">
			{m.supervision_home_controls()}
		</h2>
		<div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
			<section class="paper gap-2" aria-label={m.supervision_overview_policies()}>
				<h3 class="text-base font-semibold">{m.supervision_overview_policies()}</h3>
				{#if overview}
					<div class="flex gap-4">
						<div class="flex flex-col">
							<span class="text-2xl font-semibold tabular-nums">{overview.policies.enforce}</span>
							<span class="text-muted-content text-xs">
								{m.supervision_overview_policies_enforce({ count: overview.policies.enforce })}
							</span>
						</div>
						<div class="flex flex-col">
							<span class="text-2xl font-semibold tabular-nums">{overview.policies.shadow}</span>
							<span class="text-muted-content text-xs">
								{m.supervision_overview_policies_shadow({ count: overview.policies.shadow })}
							</span>
						</div>
					</div>
				{/if}
				<a href={resolve('/policies')} class="text-link w-fit text-sm">
					{m.supervision_overview_manage_policies()}
				</a>
			</section>

			<section class="paper gap-2" aria-label={m.supervision_kill_active()}>
				<div class="flex items-center justify-between gap-2">
					<h3 class="text-base font-semibold">{m.supervision_kill_active()}</h3>
					{#if !readonly}
						<button
							type="button"
							class="btn btn-error btn-sm text-xs"
							onclick={() => killDialog?.open()}
						>
							<OctagonX class="size-4" />
							{m.supervision_kill_switch()}
						</button>
					{/if}
				</div>
				<p class="text-muted-content text-xs">{m.supervision_kill_switch_description()}</p>
				<ActiveKills {kills} onRevoked={(id) => (kills = kills.filter((k) => k.id !== id))} />
			</section>

			<section class="paper gap-2" aria-label={m.supervision_home_redteam()}>
				<div class="flex items-center justify-between gap-2">
					<h3 class="text-base font-semibold">{m.supervision_home_redteam()}</h3>
					{#if !readonly}
						<button
							type="button"
							class="btn btn-secondary btn-sm text-xs"
							disabled={runningRedTeam}
							onclick={runRed}
						>
							{#if runningRedTeam}<Loading class="size-4" />{:else}<Swords class="size-4" />{/if}
							{m.supervision_redteam_run()}
						</button>
					{/if}
				</div>
				<p class="text-muted-content text-xs">{m.supervision_redteam_description()}</p>
			</section>
		</div>

		{#if redTeamError}
			<div class="notification-error rounded-md p-3 text-sm" role="alert">{redTeamError}</div>
		{/if}
		{#if redTeam}
			<RedTeamScorecard result={redTeam} />
		{/if}
	</div>
</Layout>

{#snippet navActions()}
	<div class="join" role="group" aria-label={m.supervision_sessions_filter_range()}>
		{#each [24, 168] as h (h)}
			<button
				type="button"
				class={twMerge(
					'btn btn-sm join-item text-xs',
					hours === h ? 'btn-primary' : 'btn-secondary'
				)}
				aria-pressed={hours === h}
				onclick={() => setHours(h)}
			>
				{h === 24 ? m.supervision_sessions_range_24h() : m.supervision_sessions_range_7d()}
			</button>
		{/each}
	</div>
{/snippet}

<KillSwitchDialog bind:this={killDialog} onCreated={(kill) => (kills = [kill, ...kills])} />
