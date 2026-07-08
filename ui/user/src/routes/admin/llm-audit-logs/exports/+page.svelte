<script lang="ts">
	import { browser } from '$app/environment';
	import { afterNavigate, beforeNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import CreateAuditLogExportForm from '$lib/components/admin/audit-log-exports/CreateAuditLogExportForm.svelte';
	import CreateScheduledExportForm from '$lib/components/admin/audit-log-exports/CreateScheduleForm.svelte';
	import StorageCredentialsForm from '$lib/components/admin/audit-log-exports/StorageCredentialsForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		AdminService,
		type AuditLogExport,
		type LLMAuditLogExport,
		type ScheduledAuditLogExport,
		type ScheduledLLMAuditLogExport
	} from '$lib/services';
	import { profile } from '$lib/stores';
	import { goto, replaceState } from '$lib/url';
	import ExportsView from '../../audit-logs/exports/ExportsView.svelte';
	import ScheduledExportsView from '../../audit-logs/exports/ScheduledExportsView.svelte';
	import { Plus, Settings } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type View = 'exports' | 'scheduled';
	type FormType = 'export' | 'scheduled' | 'storage';

	const RELOAD_DURATION_MS = 10 * 1000;
	const basePath = '/admin/llm-audit-logs/exports';

	let view = $state<View>((page.url.searchParams.get('view') as View) || 'exports');
	let query = $state('');
	let showForm = $state<FormType | null>(null);

	let exportsViewRef = $state<ExportsView>();
	let scheduledExportsViewRef = $state<ScheduledExportsView>();

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let createdExport = $state<LLMAuditLogExport | null>(null);
	let setTimeoutIds: ReturnType<typeof setTimeout>[] = [];

	onMount(async () => {
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

	beforeNavigate(cleanup);

	function cleanup() {
		setTimeoutIds.forEach(clearTimeout);
		setTimeoutIds = [];
	}

	async function switchView(newView: View) {
		view = newView;
		page.url.searchParams.set('view', newView);
		replaceState(page.url, {});
	}

	async function reload() {
		if (view === 'exports') {
			return exportsViewRef?.reload?.(false);
		}

		return scheduledExportsViewRef?.reload?.(false);
	}

	async function reloadAndCheckExportStatus() {
		const exports = (await reload()) ?? [];
		const exp = exports.find((d) => d.id === createdExport?.id);

		if (
			createdExport &&
			exp &&
			'state' in exp &&
			(exp.state === createdExport.state || exp.state === 'running')
		) {
			cleanup();
			const id = setTimeout(() => {
				reloadAndCheckExportStatus();
			}, RELOAD_DURATION_MS);
			setTimeoutIds.push(id);
		}
	}

	async function openForm(formType: FormType) {
		if (formType === 'export' || formType === 'scheduled') {
			try {
				const response = await AdminService.getStorageCredentials();
				if (response.provider) {
					showForm = formType;
					goto(`${basePath}?form=${formType}`, { replaceState: false });
				} else {
					showForm = 'storage';
					goto(`${basePath}?form=storage&next=${formType}`, { replaceState: false });
				}
			} catch (error) {
				console.error('Failed to get storage credentials:', error);
				showForm = 'storage';
				goto(`${basePath}?form=storage&next=${formType}`, { replaceState: false });
			}
		} else {
			showForm = formType;
			goto(`${basePath}?form=${formType}`, { replaceState: false });
		}
	}

	function closeForm() {
		showForm = null;
		goto(basePath, { replaceState: false });
	}

	async function handleFormSuccess(
		item?: AuditLogExport | LLMAuditLogExport | ScheduledAuditLogExport | ScheduledLLMAuditLogExport
	) {
		createdExport = item && 'startTime' in item ? ({ ...item } as LLMAuditLogExport) : null;
		showForm = null;
		await goto(basePath, { replaceState: false });

		if (item) {
			const id = setTimeout(() => {
				reloadAndCheckExportStatus();
			}, 1000);
			setTimeoutIds.push(id);
		}
	}

	function handleStorageSuccess() {
		const nextForm = page.url.searchParams.get('next') as FormType;
		if (nextForm) {
			showForm = nextForm;
			const url = new URL(page.url);
			url.searchParams.set('form', nextForm);
			url.searchParams.delete('next');
			goto(url, { replaceState: false });
		} else {
			showForm = null;
			goto(basePath, { replaceState: false });
		}
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout classes={{ navbar: 'bg-base-200' }} showBackButton title="LLM Audit Log Exports">
	<div class="flex min-h-full flex-col gap-8" in:fade>
		{#if showForm}
			{@render formScreen()}
		{:else}
			{@render mainContent()}
		{/if}
	</div>
</Layout>

{#snippet mainContent()}
	<div
		class="flex min-h-full flex-col"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<div
			class="mb-4 flex flex-col items-center justify-start md:mb-8 md:flex-row md:justify-between"
		>
			<div class="flex items-center gap-2">
				<h1 class="text-2xl font-semibold">LLM Audit Log Exports</h1>
			</div>

			<div class="mt-4 w-full shrink-0 md:mt-0 md:w-fit">
				<div class="flex gap-2">
					{#if !isAdminReadonly}
						<button
							class="btn btn-secondary flex items-center gap-1 text-sm font-normal"
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

		<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full pb-1">
			<div class="mb-2">
				<Search
					class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
					onChange={(val) => (query = val)}
					placeholder={view === 'exports' ? 'Search exports...' : 'Search schedules...'}
				/>
			</div>
		</div>

		<div class="dark:bg-base-300 bg-base-100 rounded-t-md shadow-sm">
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
				<ExportsView bind:this={exportsViewRef} {query} logType="llm" />
			{:else if view === 'scheduled'}
				<ScheduledExportsView
					bind:this={scheduledExportsViewRef}
					{query}
					readonly={isAdminReadonly}
					logType="llm"
				/>
			{/if}
		</div>
	</div>
{/snippet}

{#snippet formScreen()}
	<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		{#if showForm === 'export'}
			<CreateAuditLogExportForm onCancel={closeForm} onSubmit={handleFormSuccess} logType="llm" />
		{:else if showForm === 'scheduled'}
			<CreateScheduledExportForm onCancel={closeForm} onSubmit={handleFormSuccess} logType="llm" />
		{:else if showForm === 'storage'}
			<StorageCredentialsForm onCancel={closeForm} onSubmit={handleStorageSuccess} />
		{/if}
	</div>
{/snippet}

{#snippet addButton()}
	<DotDotDot class="btn btn-block btn-primary w-full text-sm md:w-fit" placement="bottom">
		{#snippet icon()}
			<span class="flex items-center justify-center gap-1">
				<Plus class="size-4" /> Add Export
			</span>
		{/snippet}
		<button class="menu-button" onclick={() => openForm('export')}> Create One-time Export </button>
		<button class="menu-button" onclick={() => openForm('scheduled')}>
			Create Export Schedule
		</button>
	</DotDotDot>
{/snippet}

<svelte:head>
	<title>Obot | LLM Audit Log Exports</title>
</svelte:head>
