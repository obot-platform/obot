<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import CreateScheduleForm from '$lib/components/admin/audit-log-exports/CreateScheduleForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService } from '$lib/services';
	import type { ScheduledAuditLogExport } from '$lib/services/admin/types';
	import { goto } from '$lib/url';
	import { TriangleAlert } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';

	const scheduleId = page.params.id;
	let loading = $state(true);
	let error = $state('');
	let scheduleData = $state<ScheduledAuditLogExport | null>(null);

	onMount(async () => {
		if (scheduleId) {
			try {
				scheduleData = await AdminService.getScheduledAuditLogExport(scheduleId);
			} catch (err) {
				error = err instanceof Error ? err.message : 'Failed to load scheduled export';
			} finally {
				loading = false;
			}
		}
	});

	function handleCancel() {
		goto('/admin/audit-logs/exports');
	}

	function handleSave() {
		goto('/admin/audit-logs/exports');
	}

	const duration = PAGE_TRANSITION_DURATION;
	let title = $derived(scheduleData?.name ?? 'Edit Scheduled Export');
</script>

<Layout classes={{ navbar: 'bg-base-200' }} {title} showBackButton>
	<div class="flex min-h-full flex-col gap-8" in:fade>
		{#if loading}
			<div class="flex items-center justify-center py-8">
				<Loading class="size-8" />
				<span class="ml-2 text-lg">Loading scheduled export details...</span>
			</div>
		{:else if error}
			<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
				<div class="rounded-md bg-error/10">
					<div class="flex items-center gap-2">
						<TriangleAlert class="size-5 text-error" />
						<span class="text-sm font-medium text-error">Error loading scheduled export</span>
					</div>
					<p class="mt-2 text-sm text-error">{error}</p>
				</div>
			</div>
		{:else if scheduleData}
			<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
				<CreateScheduleForm
					mode="edit"
					initialData={scheduleData}
					onCancel={handleCancel}
					onSubmit={handleSave}
				/>
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
