<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import CreateAuditLogExportForm from '$lib/components/admin/audit-log-exports/CreateAuditLogExportForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService } from '$lib/services';
	import type { AuditLogExport } from '$lib/services/admin/types';
	import { TriangleAlert } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';

	const exportId = page.params.id;
	let loading = $state(true);
	let error = $state('');
	let exportData = $state<AuditLogExport>();

	onMount(async () => {
		if (!exportId) return;
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

<Layout classes={{ navbar: 'bg-base-200' }} {title} showBackButton>
	<div class="flex min-h-full flex-col gap-8" in:fade>
		{#if loading}
			<div class="flex items-center justify-center py-8">
				<Loading class="size-8" />
				<span class="ml-2 text-lg">Loading export details...</span>
			</div>
		{:else if error}
			<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
				<div class="rounded-md bg-error/10">
					<div class="flex items-center gap-2">
						<TriangleAlert class="size-5 text-error" />
						<span class="text-sm font-medium text-error">Error loading export</span>
					</div>
					<p class="mt-2 text-sm text-error">{error}</p>
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
