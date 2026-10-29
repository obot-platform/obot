<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		getProvenanceSettings,
		surfaceLabel,
		updateProvenanceSettings,
		type SupervisorStatus
	} from '$lib/services/provenance';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { formatTimeAgo } from '$lib/time';
	import { resolve } from '$lib/url';
	import SupervisorAvatar from './SupervisorAvatar.svelte';
	import { Power } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	// The Supervisor's presence: whether it is on, what it watches, when it last decided and
	// what it did today. When it is off, say so plainly and offer the way to turn it on.

	interface Props {
		status?: SupervisorStatus;
		loading?: boolean;
		onChanged?: () => void;
	}

	let { status, loading = false, onChanged }: Props = $props();

	let readonly = $derived(profile.current.isAdminReadonly?.());
	let enabling = $state(false);
	let error = $state('');

	// Re-render relative times every few seconds so "last decision" stays honest.
	let now = $state(Date.now());
	onMount(() => {
		const timer = setInterval(() => (now = Date.now()), 5_000);
		return () => clearInterval(timer);
	});

	function ago(ts: string | null | undefined, _tick: number) {
		return ts ? formatTimeAgo(ts).relativeTime : '';
	}

	async function enable() {
		enabling = true;
		error = '';
		try {
			const settings = await getProvenanceSettings({ dontLogErrors: true });
			await updateProvenanceSettings(
				{ ...settings, supervisorEnabled: true },
				{ dontLogErrors: true }
			);
			success.add(m.supervision_presence_enabled_toast());
			onChanged?.();
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			enabling = false;
		}
	}

	let tiles = $derived(
		status
			? [
					{
						id: 'decided',
						label: m.supervision_presence_today_decided(),
						value: status.today.decided,
						color: ''
					},
					{
						id: 'judged',
						label: m.supervision_presence_today_judged(),
						value: status.today.judged,
						color: 'text-primary'
					},
					{
						id: 'held',
						label: m.supervision_presence_today_held(),
						value: status.today.held,
						color: 'text-warning'
					},
					{
						id: 'denied',
						label: m.supervision_presence_today_denied(),
						value: status.today.denied,
						color: 'text-error'
					}
				]
			: []
	);
</script>

<section
	class={twMerge(
		'paper flex-row flex-wrap items-center justify-between gap-4',
		status?.enabled && 'from-primary/10 bg-linear-to-r to-transparent'
	)}
	aria-label={m.supervision_presence_name()}
>
	{#if loading && !status}
		<div class="flex items-center gap-3">
			<SupervisorAvatar size="lg" active={false} />
			<Loading class="size-5" />
		</div>
	{:else if status}
		<div class="flex min-w-0 items-center gap-4">
			<SupervisorAvatar size="lg" active={status.enabled} pulse={status.enabled} />
			<div class="flex min-w-0 flex-col gap-1">
				<div class="flex flex-wrap items-center gap-2">
					<h2 class="text-lg font-semibold">{m.supervision_presence_name()}</h2>
					{#if status.enabled}
						<span class="badge badge-sm badge-soft badge-success gap-1">
							<span class="bg-success size-1.5 animate-pulse rounded-full"></span>
							{m.supervision_presence_active()}
						</span>
					{:else}
						<span class="badge badge-sm badge-ghost">{m.supervision_presence_off()}</span>
					{/if}
				</div>

				{#if status.enabled}
					<p class="text-sm">
						{m.supervision_presence_watching({
							sessions: status.watching.sessions,
							surfaces: status.watching.surfaces.length
						})}
						{#if status.watching.openSessions}
							<span class="text-muted-content">
								{m.supervision_presence_open({ count: status.watching.openSessions })}
							</span>
						{/if}
					</p>
					<div class="text-muted-content flex flex-wrap gap-x-3 gap-y-1 text-xs">
						<span>
							{status.lastDecisionAt
								? m.supervision_presence_last_decision({ time: ago(status.lastDecisionAt, now) })
								: m.supervision_presence_no_decision()}
						</span>
						{#if status.lastJudgementAt}
							<span>
								{m.supervision_presence_last_judgement({ time: ago(status.lastJudgementAt, now) })}
							</span>
						{/if}
						<span class="max-w-md truncate" title={status.model}>
							{m.supervision_overview_supervisor_model({
								model: status.model || m.supervision_overview_supervisor_default_model()
							})}
						</span>
						{#if status.deciderModel && status.deciderModel !== status.model}
							<span>{m.supervision_presence_decider({ model: status.deciderModel })}</span>
						{/if}
					</div>
					{#if status.watching.surfaces.length}
						<span class="text-muted-content text-xs">
							{status.watching.surfaces.map(surfaceLabel).join(' · ')}
						</span>
					{/if}
				{:else}
					<p class="max-w-2xl text-sm">{m.supervision_presence_off_body()}</p>
					{#if error}
						<p class="text-error text-xs" role="alert">{error}</p>
					{/if}
					{#if !status.configured}
						<p class="text-muted-content text-xs">
							{m.supervision_presence_unconfigured()}
							<a href={resolve('/admin/model-providers')} class="text-link">
								{m.supervision_presence_configure()}
							</a>
						</p>
					{:else if !readonly}
						<button
							type="button"
							class="btn btn-primary btn-sm w-fit text-xs"
							disabled={enabling}
							onclick={enable}
						>
							{#if enabling}<Loading class="size-4" />{:else}<Power class="size-4" />{/if}
							{m.supervision_presence_enable()}
						</button>
					{/if}
				{/if}
			</div>
		</div>

		{#if status.enabled || status.today.decided}
			<dl class="grid grid-cols-2 gap-2 sm:grid-cols-4">
				{#each tiles as tile (tile.id)}
					<div class="bg-base-100/70 dark:bg-base-300 flex min-w-24 flex-col rounded-md p-2">
						<dt class="text-muted-content text-xs">{tile.label}</dt>
						<dd class={twMerge('text-2xl font-semibold tabular-nums', tile.color)}>
							{tile.value}
						</dd>
					</div>
				{/each}
			</dl>
		{/if}
	{/if}
</section>
