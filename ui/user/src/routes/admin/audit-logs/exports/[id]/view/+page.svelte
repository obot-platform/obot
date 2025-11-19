<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { AdminService } from '$lib/services';
	import type { AuditLogExport } from '$lib/services/admin/types';
	import CreateAuditLogExportForm from '$lib/components/admin/audit-log-exports/CreateAuditLogExportForm.svelte';
	import { LoaderCircle } from 'lucide-svelte';
	import { fade, fly } from 'svelte/transition';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';

	const exportId = page.params.id;
	let loading = $state(true);
	let error = $state('');
	let exportData = $state<AuditLogExport>();

	onMount(async () => {
		try {
			exportData = (await AdminService.getAuditLogExport(exportId)) as AuditLogExport;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load export';
		} finally {
			loading = false;
		}
	});

	const duration = PAGE_TRANSITION_DURATION;
	let title = $derived(exportData?.name ?? 'View Export');
</script>

<Layout classes={{ navbar: 'bg-surface1' }} {title} showBackButton>
	<div class="flex min-h-full flex-col gap-8 pt-4" in:fade>
		{#if loading}
			<div class="flex items-center justify-center py-8">
				<LoaderCircle class="text-primary size-8 animate-spin" />
				<span class="ml-2 text-lg">Loading export details...</span>
			</div>
		{:else if error}
			<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
				<div class="rounded-md bg-red-50 p-4 dark:bg-red-950/50">
					<div class="flex items-center gap-2">
						<svg class="size-5 text-red-600" fill="currentColor" viewBox="0 0 20 20">
							<path
								fill-rule="evenodd"
								d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
								clip-rule="evenodd"
							></path>
						</svg>
						<span class="text-sm font-medium text-red-800 dark:text-red-200"
							>Error loading export</span
						>
					</div>
					<p class="mt-2 text-sm text-red-700 dark:text-red-300">{error}</p>
				</div>
			</div>
		{:else if exportData}
			<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
				<CreateAuditLogExportForm
					mode="view"
					initialData={exportData}
					onCancel={() => window.history.back()}
					onSubmit={() => {}}
				/>
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
