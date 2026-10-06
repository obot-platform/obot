<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import CreateScheduleForm from '$lib/components/admin/audit-log-exports/CreateScheduleForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService } from '$lib/services';
	import type { ScheduledAuditLogExport } from '$lib/services/admin/types';
	import { goto } from '$lib/url';
	import { TriangleAlert } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';

	const scheduleId = page.params.id;
	let loading = $state(true);
	let error = $state('');
	let scheduleData = $state<ScheduledAuditLogExport | null>(null);

	onMount(async () => {
		if (!scheduleId) {
			error = m.audit_usage_audit_logs_scheduled_export_id_required();
			loading = false;
			return;
		}

		try {
			scheduleData = await AdminService.getScheduledAuditLogExport(scheduleId);
		} catch (err) {
			error =
				err instanceof Error
					? err.message
					: m.audit_usage_audit_logs_failed_load_scheduled_export();
		} finally {
			loading = false;
		}
	});

	function handleNavigate() {
		goto('/audit-logs/llm/exports');
	}

	const duration = PAGE_TRANSITION_DURATION;
	let title = $derived(scheduleData?.name ?? m.audit_usage_audit_logs_edit_scheduled_export());
</script>

<Layout classes={{ navbar: 'bg-base-200' }} {title} showBackButton>
	<div class="flex min-h-full flex-col gap-8" in:fade>
		{#if loading}
			<div class="flex items-center justify-center py-8">
				<Loading class="size-8" />
				<span class="ml-2 text-lg"
					>{m.audit_usage_audit_logs_loading_scheduled_export_details()}</span
				>
			</div>
		{:else if error}
			<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
				<div class="rounded-md bg-error/10">
					<div class="flex items-center gap-2">
						<TriangleAlert class="size-5 text-error" />
						<span class="text-sm font-medium text-error"
							>{m.audit_usage_audit_logs_error_loading_scheduled_export()}</span
						>
					</div>
					<p class="mt-2 text-sm text-error">{error}</p>
				</div>
			</div>
		{:else if scheduleData}
			<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
				<CreateScheduleForm
					mode="edit"
					initialData={scheduleData}
					onCancel={handleNavigate}
					onSubmit={handleNavigate}
					logType="llm"
				/>
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>{m.audit_usage_audit_logs_obot_title({ title })}</title>
</svelte:head>
