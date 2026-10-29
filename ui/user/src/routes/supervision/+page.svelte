<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import HorizontalBarList from '$lib/components/graph/HorizontalBarList.svelte';
	import ActiveKills from '$lib/components/supervision/ActiveKills.svelte';
	import KillSwitchDialog from '$lib/components/supervision/KillSwitchDialog.svelte';
	import LabelBadges from '$lib/components/supervision/LabelBadges.svelte';
	import RedTeamScorecard from '$lib/components/supervision/RedTeamScorecard.svelte';
	import RiskPill from '$lib/components/supervision/RiskPill.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		effectLabel,
		getOverview,
		listKills,
		runRedTeam,
		sessionDisplayName,
		SURFACES,
		surfaceLabel,
		type Kill,
		type Overview,
		type RedTeamResult
	} from '$lib/services/provenance';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { resolve } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { CircleHelp, OctagonX, Sparkles, Swords } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	// The Supervision overview: coverage per surface, decisions, the riskiest sessions, policy
	// and Supervisor status, the kill switch and the red team scorecard.

	let hours = $state(24);
	let overview = $state<Overview>();
	let kills = $state<Kill[]>([]);
	let loading = $state(true);
	let error = $state('');
	let killDialog = $state<ReturnType<typeof KillSwitchDialog>>();
	let redTeam = $state<RedTeamResult>();
	let runningRedTeam = $state(false);
	let redTeamError = $state('');

	let readonly = $derived(profile.current.isAdminReadonly?.());

	async function load() {
		loading = true;
		error = '';
		try {
			[overview, kills] = await Promise.all([
				getOverview(hours, { dontLogErrors: true }),
				listKills({ dontLogErrors: true }).catch((): Kill[] => [])
			]);
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			loading = false;
		}
	}

	onMount(load);

	async function setHours(h: number) {
		hours = h;
		await load();
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

	let decisionTiles = $derived(
		overview
			? [
					{
						id: 'allowed',
						label: m.supervision_overview_allowed(),
						value: overview.decisions.allowed,
						color: 'text-success'
					},
					{
						id: 'denied',
						label: m.supervision_overview_denied(),
						value: overview.decisions.denied,
						color: 'text-error'
					},
					{
						id: 'shadowed',
						label: m.supervision_overview_shadowed(),
						value: overview.decisions.shadowed,
						color: 'text-warning'
					},
					{
						id: 'pending',
						label: m.supervision_overview_pending_approvals(),
						value: overview.decisions.approvals_pending,
						color: 'text-primary',
						href: '/approvals'
					}
				]
			: []
	);

	let bySurface = $derived(
		overview
			? Object.entries(overview.bySurface)
					.map(([surface, count]) => ({ label: surfaceLabel(surface), value: count ?? 0 }))
					.sort((a, b) => b.value - a.value)
			: []
	);
	let byEffect = $derived(
		overview
			? Object.entries(overview.byEffect)
					.map(([effect, count]) => ({ label: effectLabel(effect), value: count ?? 0 }))
					.sort((a, b) => b.value - a.value)
			: []
	);

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

		{#if loading && !overview}
			<Skeleton type="card" class="min-h-40 w-full" />
			<Skeleton type="card" class="min-h-40 w-full" />
			<Skeleton type="table" count={4} />
		{:else if overview}
			<!-- Coverage -->
			<section class="paper gap-3" aria-label={m.supervision_overview_coverage()}>
				<div class="flex flex-wrap items-baseline justify-between gap-2">
					<h2 class="text-base font-semibold">{m.supervision_overview_coverage()}</h2>
					<span class="text-muted-content text-xs">
						{m.supervision_overview_coverage_description()} · {m.supervision_hours_window({
							hours
						})}
					</span>
				</div>
				<div class="grid grid-cols-2 gap-2 md:grid-cols-4 lg:grid-cols-7">
					<div class="bg-base-200 dark:bg-base-300 flex flex-col rounded-md p-3">
						<span class="text-muted-content text-xs">{m.supervision_overview_sessions()}</span>
						<span class="text-2xl font-semibold tabular-nums">{overview.coverage.sessions}</span>
					</div>
					{#each SURFACES as surface (surface)}
						{@const count = overview.coverage.surfaces[surface] ?? 0}
						<div
							class={twMerge(
								'bg-base-200 dark:bg-base-300 flex flex-col rounded-md p-3',
								count === 0 && 'opacity-60'
							)}
						>
							<span class="text-muted-content truncate text-xs">{surfaceLabel(surface)}</span>
							<span class="text-2xl font-semibold tabular-nums">{count}</span>
						</div>
					{/each}
					<div
						class={twMerge(
							'flex flex-col rounded-md p-3',
							overview.coverage.blind > 0 ? 'bg-warning/15' : 'bg-base-200 dark:bg-base-300'
						)}
						title={m.supervision_overview_blind_sessions_help()}
					>
						<span class="text-muted-content flex items-center gap-1 text-xs">
							{m.supervision_overview_blind_sessions()}
							<CircleHelp class="size-3" />
						</span>
						<span class="text-2xl font-semibold tabular-nums">{overview.coverage.blind}</span>
					</div>
				</div>
			</section>

			<!-- Decisions -->
			<section class="paper gap-3" aria-label={m.supervision_overview_decisions()}>
				<h2 class="text-base font-semibold">{m.supervision_overview_decisions()}</h2>
				<div class="grid grid-cols-2 gap-2 md:grid-cols-4">
					{#each decisionTiles as tile (tile.id)}
						{#if tile.href}
							<a
								href={resolve(tile.href as `/${string}`)}
								class="bg-base-200 dark:bg-base-300 hover:bg-base-300 dark:hover:bg-base-400 flex flex-col rounded-md p-3 transition-colors"
							>
								<span class="text-muted-content text-xs">{tile.label}</span>
								<span class={twMerge('text-2xl font-semibold tabular-nums', tile.color)}
									>{tile.value}</span
								>
							</a>
						{:else}
							<div class="bg-base-200 dark:bg-base-300 flex flex-col rounded-md p-3">
								<span class="text-muted-content text-xs">{tile.label}</span>
								<span class={twMerge('text-2xl font-semibold tabular-nums', tile.color)}
									>{tile.value}</span
								>
							</div>
						{/if}
					{/each}
				</div>
				<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
					<div class="flex flex-col gap-2">
						<h3 class="text-sm font-medium">{m.supervision_overview_by_surface()}</h3>
						{#if bySurface.length}
							<HorizontalBarList data={bySurface} labelKey="label" valueKey="value" />
						{:else}
							<p class="text-muted-content text-sm">{m.supervision_overview_no_decisions()}</p>
						{/if}
					</div>
					<div class="flex flex-col gap-2">
						<h3 class="text-sm font-medium">{m.supervision_overview_by_effect()}</h3>
						{#if byEffect.length}
							<HorizontalBarList data={byEffect} labelKey="label" valueKey="value" />
						{:else}
							<p class="text-muted-content text-sm">{m.supervision_overview_no_decisions()}</p>
						{/if}
					</div>
				</div>
			</section>

			<!-- Riskiest sessions -->
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

			<div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
				<!-- Policies -->
				<section class="paper gap-2" aria-label={m.supervision_overview_policies()}>
					<h2 class="text-base font-semibold">{m.supervision_overview_policies()}</h2>
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
					<a href={resolve('/policies')} class="text-link w-fit text-sm">
						{m.supervision_overview_manage_policies()}
					</a>
				</section>

				<!-- Supervisor -->
				<section class="paper gap-2" aria-label={m.supervision_overview_supervisor()}>
					<h2 class="flex items-center gap-2 text-base font-semibold">
						<Sparkles class="text-primary size-4" />
						{m.supervision_overview_supervisor()}
					</h2>
					<div class="flex items-center gap-2">
						<span
							class={twMerge(
								'badge badge-sm badge-soft',
								overview.supervisor.enabled ? 'badge-success' : 'badge-ghost'
							)}
						>
							{overview.supervisor.enabled
								? m.supervision_overview_supervisor_enabled()
								: m.supervision_overview_supervisor_disabled()}
						</span>
						<span class="text-muted-content text-xs">
							{m.supervision_overview_supervisor_model({
								model:
									overview.supervisor.model || m.supervision_overview_supervisor_default_model()
							})}
						</span>
					</div>
					<p class="text-muted-content text-xs">{m.supervision_overview_supervisor_help()}</p>
				</section>

				<!-- Kill switch -->
				<section class="paper gap-2" aria-label={m.supervision_kill_active()}>
					<div class="flex items-center justify-between gap-2">
						<h2 class="text-base font-semibold">{m.supervision_kill_active()}</h2>
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
					<ActiveKills {kills} onRevoked={(id) => (kills = kills.filter((k) => k.id !== id))} />
				</section>
			</div>
		{/if}

		{#if redTeamError}
			<div class="notification-error rounded-md p-3 text-sm" role="alert">{redTeamError}</div>
		{/if}
		{#if redTeam}
			<RedTeamScorecard result={redTeam} />
		{/if}
	</div>
</Layout>

{#snippet navActions()}
	<div class="flex items-center gap-2">
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
		{#if !readonly}
			<button
				type="button"
				class="btn btn-secondary rounded-4xl text-sm"
				disabled={runningRedTeam}
				onclick={runRed}
			>
				{#if runningRedTeam}<Loading class="size-4" />{:else}<Swords class="size-4" />{/if}
				{m.supervision_redteam_run()}
			</button>
			<button
				type="button"
				class="btn btn-error rounded-4xl text-sm"
				onclick={() => killDialog?.open()}
			>
				<OctagonX class="size-4" />
				{m.supervision_kill_switch()}
			</button>
		{/if}
	</div>
{/snippet}

<KillSwitchDialog bind:this={killDialog} onCreated={(kill) => (kills = [kill, ...kills])} />
