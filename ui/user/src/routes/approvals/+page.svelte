<script lang="ts">
	import { page } from '$app/state';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import TabLayout from '$lib/components/TabLayout.svelte';
	import ApprovalCard from '$lib/components/supervision/ApprovalCard.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import {
		APPROVAL_STATUSES,
		approvalStatusLabel,
		listApprovals,
		type Approval,
		type ApprovalStatus
	} from '$lib/services/provenance';
	import { Inbox, RefreshCw } from '@lucide/svelte';

	// The approvals inbox. Pending requests refresh on an interval so an approver sees new holds
	// without reloading; resolved tabs load once per visit.

	const REFRESH_MS = 10_000;

	let approvals = $state<Approval[]>([]);
	let loading = $state(true);
	let error = $state('');
	let generation = 0;

	let status = $derived.by<ApprovalStatus>(() => {
		const view = page.url.searchParams.get('view');
		return APPROVAL_STATUSES.includes(view as ApprovalStatus)
			? (view as ApprovalStatus)
			: 'pending';
	});
	let highlighted = $derived(page.url.searchParams.get('approval') || '');

	async function load(quiet = false) {
		const gen = ++generation;
		const forStatus = status;
		if (!quiet) loading = true;
		error = '';
		try {
			const resp = await listApprovals({ status: forStatus, limit: 100 }, { dontLogErrors: true });
			if (gen !== generation) return;
			approvals = resp.items ?? [];
		} catch (e) {
			if (gen !== generation) return;
			error = parseErrorContent(e).message;
		} finally {
			if (gen === generation) loading = false;
		}
	}

	// Load on tab change, and poll while pending is shown. setInterval, not setTimeout, so the
	// cadence is not shortened by the test harness.
	$effect(() => {
		const s = status;
		load();
		if (s !== 'pending') return;
		const timer = setInterval(() => load(true), REFRESH_MS);
		return () => clearInterval(timer);
	});

	function onResolved(resolved: Approval) {
		approvals = approvals.filter((a) => a.id !== resolved.id);
	}

	const views = APPROVAL_STATUSES.map((value) => ({
		value,
		label: approvalStatusLabel(value),
		content: list
	}));
</script>

<svelte:head>
	<title>Obot | {m.supervision_approvals_title()}</title>
</svelte:head>

<TabLayout
	title={m.supervision_approvals_title()}
	defaultView="pending"
	{views}
	rightNavActions={navActions}
/>

{#snippet navActions()}
	<button
		type="button"
		class="btn btn-secondary rounded-4xl text-sm"
		disabled={loading}
		onclick={() => load()}
	>
		<RefreshCw class="size-4 {loading ? 'animate-spin' : ''}" />
		{m.supervision_refresh()}
	</button>
{/snippet}

{#snippet list()}
	<div class="flex flex-col gap-4 pb-8">
		<p class="text-muted-content text-sm">
			{m.supervision_approvals_description()}
			{#if status === 'pending'}
				{m.supervision_approvals_auto_refresh()}
			{/if}
		</p>

		{#if error}
			<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
		{/if}

		{#if loading && approvals.length === 0}
			<Skeleton type="card" class="min-h-40 w-full" />
			<Skeleton type="card" class="min-h-40 w-full" />
		{:else if approvals.length === 0}
			<div class="mt-12 flex w-md max-w-full flex-col items-center gap-4 self-center text-center">
				<Inbox class="text-muted-content size-24 opacity-25" />
				<p class="text-muted-content text-sm">
					{m.supervision_approvals_empty({ status: approvalStatusLabel(status).toLowerCase() })}
				</p>
			</div>
		{:else}
			<div class="flex flex-col gap-3">
				{#each approvals as approval (approval.id)}
					<ApprovalCard {approval} {onResolved} highlighted={approval.id === highlighted} />
				{/each}
			</div>
		{/if}
	</div>
{/snippet}
