<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import BackLink from '$lib/components/BackLink.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { Plus, Settings } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';
	import { page } from '$app/state';
	import { replaceState, goto } from '$app/navigation';
	import { afterNavigate } from '$app/navigation';
	import { browser } from '$app/environment';
	import { profile } from '$lib/stores';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService } from '$lib/services';
	import ExportsView from './ExportsView.svelte';
	import ScheduledExportsView from './ScheduledExportsView.svelte';
	import CreateAuditLogExportForm from '$lib/components/admin/audit-log-exports/CreateAuditLogExportForm.svelte';
	import CreateScheduledExportForm from '$lib/components/admin/audit-log-exports/CreateScheduleForm.svelte';
	import StorageCredentialsForm from '$lib/components/admin/audit-log-exports/StorageCredentialsForm.svelte';

	type View = 'exports' | 'scheduled';
	type FormType = 'export' | 'scheduled' | 'storage';

	let view = $state<View>((page.url.searchParams.get('view') as View) || 'exports');
	let query = $state('');
	let showForm = $state<FormType | null>(null);

	// View component references for refreshing data
	let exportsViewRef = $state<ExportsView>();
	let scheduledExportsViewRef = $state<ScheduledExportsView>();

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	onMount(async () => {
		// Check URL parameters for form state
		const formType = page.url.searchParams.get('form') as FormType;
		if (formType) {
			showForm = formType;
		}
	});

	afterNavigate(({ to }) => {
		if (browser && to?.url) {
			const formType = to.url.searchParams.get('form') as FormType;
			if (!formType && showForm) {
				showForm = null;
			} else if (formType && !showForm) {
				showForm = formType;
			}
		}
	});

	async function switchView(newView: View) {
		view = newView;
		page.url.searchParams.set('view', newView);
		replaceState(page.url, {});
	}

	async function openForm(formType: FormType) {
		// If trying to open export or scheduled form, check if storage credentials are configured first
		if (formType === 'export' || formType === 'scheduled') {
			try {
				const response = await AdminService.getStorageCredentials();
				if (response.provider) {
					// Storage is configured, proceed to the form
					showForm = formType;
					goto(`/admin/audit-logs/exports?form=${formType}`, { replaceState: false });
				} else {
					// No storage provider configured, redirect to storage form first
					showForm = 'storage';
					goto(`/admin/audit-logs/exports?form=storage&next=${formType}`, { replaceState: false });
				}
			} catch (error) {
				// Error getting storage credentials, assume not configured and redirect to storage form
				console.error('Failed to get storage credentials:', error);
				showForm = 'storage';
				goto(`/admin/audit-logs/exports?form=storage&next=${formType}`, { replaceState: false });
			}
		} else {
			// For storage form, proceed directly
			showForm = formType;
			goto(`/admin/audit-logs/exports?form=${formType}`, { replaceState: false });
		}
	}

	function closeForm() {
		showForm = null;
		goto('/admin/audit-logs/exports', { replaceState: false });
	}

	function handleFormSuccess() {
		showForm = null;
		// Refresh the appropriate view
		if (view === 'exports') {
			exportsViewRef?.reload?.();
		} else {
			scheduledExportsViewRef?.reload?.();
		}
		goto('/admin/audit-logs/exports', { replaceState: false });
	}

	function handleStorageSuccess() {
		// Check if there's a next form to show after storage configuration
		const nextForm = page.url.searchParams.get('next') as FormType;
		if (nextForm) {
			showForm = nextForm;
			// Preserve all URL parameters when redirecting to the next form
			const url = new URL(page.url);
			url.searchParams.set('form', nextForm);
			url.searchParams.delete('next');
			goto(url.pathname + url.search, { replaceState: false });
		} else {
			showForm = null;
			goto('/admin/audit-logs/exports', { replaceState: false });
		}
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout classes={{ navbar: 'bg-surface1' }}>
	<div class="flex min-h-full flex-col gap-8 pt-4" in:fade>
		{#if showForm}
			{@render formScreen()}
		{:else}
			{@render mainContent()}
		{/if}
	</div>
</Layout>

{#snippet mainContent()}
	<BackLink fromURL="audit-logs" currentLabel="Audit Log Exports" />
	<div
		class="flex min-h-full flex-col"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<div
			class="mb-4 flex flex-col items-center justify-start md:mb-8 md:flex-row md:justify-between"
		>
			<div class="flex items-center gap-2">
				<h1 class="text-2xl font-semibold">Audit Log Exports</h1>
			</div>

			<div class="mt-4 w-full flex-shrink-0 md:mt-0 md:w-fit">
				<div class="flex gap-2">
					{#if !isAdminReadonly}
						<button
							class="button-secondary flex items-center gap-1 text-sm font-normal"
							onclick={() => openForm('storage')}
						>
							<Settings class="size-4" />
							Configure Storage
						</button>
					{/if}
					{@render addButton()}
				</div>
			</div>
		</div>

		<div class="bg-surface1 sticky top-16 left-0 z-20 w-full pb-1 dark:bg-black">
			<div class="mb-2">
				<Search
					class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
					onChange={(val) => (query = val)}
					placeholder={view === 'exports' ? 'Search exports...' : 'Search schedules...'}
				/>
			</div>
		</div>

		<div class="dark:bg-surface2 rounded-t-md bg-white shadow-sm">
			<div class="flex">
				<button
					class={twMerge('page-tab', view === 'exports' && 'page-tab-active')}
					onclick={() => switchView('exports')}
				>
					Exports
				</button>
				<button
					class={twMerge('page-tab', view === 'scheduled' && 'page-tab-active')}
					onclick={() => switchView('scheduled')}
				>
					Export Schedules
				</button>
			</div>

			{#if view === 'exports'}
				<ExportsView bind:this={exportsViewRef} {query} />
			{:else if view === 'scheduled'}
				<ScheduledExportsView
					bind:this={scheduledExportsViewRef}
					{query}
					readonly={isAdminReadonly}
				/>
			{/if}
		</div>
	</div>
{/snippet}

{#snippet formScreen()}
	{@const currentLabel =
		showForm === 'export'
			? 'Create Export'
			: showForm === 'scheduled'
				? 'Create Export Schedule'
				: 'Configure Storage Credentials'}
	<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		<BackLink fromURL="audit-logs-exports" {currentLabel} />
		{#if showForm === 'export'}
			<CreateAuditLogExportForm onCancel={closeForm} onSubmit={handleFormSuccess} />
		{:else if showForm === 'scheduled'}
			<CreateScheduledExportForm onCancel={closeForm} onSubmit={handleFormSuccess} />
		{:else if showForm === 'storage'}
			<StorageCredentialsForm onCancel={closeForm} onSubmit={handleStorageSuccess} />
		{/if}
	</div>
{/snippet}

{#snippet addButton()}
	<DotDotDot class="button-primary w-full text-sm md:w-fit" placement="bottom">
		{#snippet icon()}
			<span class="flex items-center justify-center gap-1">
				<Plus class="size-4" /> Add Export
			</span>
		{/snippet}
		<div class="default-dialog flex min-w-max flex-col p-2">
			<button class="menu-button" onclick={() => openForm('export')}>
				Create One-time Export
			</button>
			<button class="menu-button" onclick={() => openForm('scheduled')}>
				Create Export Schedule
			</button>
		</div>
	</DotDotDot>
{/snippet}

<svelte:head>
	<title>Obot | Audit Log Exports</title>
</svelte:head>
