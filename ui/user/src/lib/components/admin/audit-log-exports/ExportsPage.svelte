<script lang="ts">
	import { browser } from '$app/env';
	import { afterNavigate, beforeNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import { canManageAuditExports } from '$lib/auditlogs';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import {
		AdminService,
		type AuditLogExport,
		type AuditLogType,
		type ScheduledAuditLogExport
	} from '$lib/services';
	import { profile } from '$lib/stores';
	import { goto } from '$lib/url';
	import CreateAuditLogExportForm from './CreateAuditLogExportForm.svelte';
	import CreateScheduledExportForm from './CreateScheduleForm.svelte';
	import ExportsView from './ExportsView.svelte';
	import ScheduledExportsView from './ScheduledExportsView.svelte';
	import StorageCredentialsForm from './StorageCredentialsForm.svelte';
	import { Plus, Settings } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	// The exports page of one kind of audit log, at /audit-logs/<logType>/exports.
	interface Props {
		logType: Exclude<AuditLogType, 'mcp'>;
		title: string;
		pageTitle: string;
	}

	let { logType, title, pageTitle }: Props = $props();

	type View = 'exports' | 'scheduled';
	type FormType = 'export' | 'scheduled' | 'storage';

	const RELOAD_DURATION_MS = 10 * 1000;
	const basePath = $derived(`/audit-logs/${logType}/exports`);

	let view = $state<View>((page.url.searchParams.get('view') as View) || 'exports');
	let query = $state('');
	let showForm = $state<FormType | null>(null);

	let exportsViewRef = $state<ExportsView>();
	let scheduledExportsViewRef = $state<ScheduledExportsView>();

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let canManage = $derived(canManageAuditExports(profile.current.groups));
	let createdExport = $state<AuditLogExport | null>(null);
	let setTimeoutIds: ReturnType<typeof setTimeout>[] = [];

	onMount(async () => {
		const formType = page.url.searchParams.get('form') as FormType;
		if (formType) {
			showForm = formType;
		}
	});

	afterNavigate(({ to }) => {
		if (browser && to?.url) {
			showForm = (to.url.searchParams.get('form') as FormType) || null;
		}
	});

	beforeNavigate(cleanup);

	function cleanup() {
		setTimeoutIds.forEach(clearTimeout);
		setTimeoutIds = [];
	}

	async function switchView(newView: View) {
		view = newView;
		const updatedUrl = new URL(page.url.toString());
		updatedUrl.searchParams.set('view', newView);
		goto(updatedUrl, { reset: false, replace: true });
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

	async function handleFormSuccess(item?: AuditLogExport | ScheduledAuditLogExport) {
		createdExport = item && 'startTime' in item ? { ...item } : null;
		showForm = null;
		await goto(basePath, { replaceState: false });

		if (createdExport) {
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
			const url = new URL(page.url.toString());
			url.searchParams.set('form', nextForm);
			url.searchParams.delete('next');
			goto(url);
		} else {
			showForm = null;
			goto(basePath);
		}
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout showBackButton {title}>
	<div class="flex min-h-full flex-col gap-8" in:fade>
		{#if canManage && showForm}
			{@render formScreen()}
		{:else}
			{@render mainContent()}
		{/if}
	</div>

	{#snippet rightNavActions()}
		<div class="flex gap-2">
			{#if canManage && !isAdminReadonly}
				<button
					class="btn btn-secondary flex items-center gap-1 text-sm font-normal"
					onclick={() => openForm('storage')}
				>
					<Settings class="size-4" />
					{m.audit_usage_audit_logs_configure_storage()}
				</button>
			{/if}
			{#if canManage}
				{@render addButton()}
			{/if}
		</div>
	{/snippet}
</Layout>

{#snippet mainContent()}
	<div
		class="flex min-h-full flex-col"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full pb-1">
			<div class="mb-2">
				<Search
					class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
					onChange={(val) => (query = val)}
					placeholder={view === 'exports'
						? m.audit_usage_audit_logs_search_exports()
						: m.audit_usage_audit_logs_search_schedules()}
				/>
			</div>
		</div>

		<div class="dark:bg-base-300 bg-base-100 rounded-t-md shadow-sm">
			<div class="flex">
				<button
					class={twMerge('page-tab', view === 'exports' && 'page-tab-active')}
					onclick={() => switchView('exports')}
				>
					{m.audit_usage_audit_logs_tab_exports()}
				</button>
				<button
					class={twMerge('page-tab', view === 'scheduled' && 'page-tab-active')}
					onclick={() => switchView('scheduled')}
				>
					{m.audit_usage_audit_logs_tab_export_schedules()}
				</button>
			</div>

			{#if view === 'exports'}
				<ExportsView bind:this={exportsViewRef} {query} {logType} readonly={!canManage} />
			{:else if view === 'scheduled'}
				<ScheduledExportsView
					bind:this={scheduledExportsViewRef}
					{query}
					readonly={isAdminReadonly}
					{logType}
				/>
			{/if}
		</div>
	</div>
{/snippet}

{#snippet formScreen()}
	<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		{#if showForm === 'export'}
			<CreateAuditLogExportForm onCancel={closeForm} onSubmit={handleFormSuccess} {logType} />
		{:else if showForm === 'scheduled'}
			<CreateScheduledExportForm onCancel={closeForm} onSubmit={handleFormSuccess} {logType} />
		{:else if showForm === 'storage'}
			<StorageCredentialsForm onCancel={closeForm} onSubmit={handleStorageSuccess} />
		{/if}
	</div>
{/snippet}

{#snippet addButton()}
	<DotDotDot class="btn btn-block btn-primary w-full text-sm md:w-fit" placement="bottom">
		{#snippet icon()}
			<span class="flex items-center justify-center gap-1">
				<Plus class="size-4" />
				{m.audit_usage_audit_logs_add_export()}
			</span>
		{/snippet}
		<button class="menu-button" onclick={() => openForm('export')}>
			{m.audit_usage_audit_logs_create_one_time_export()}
		</button>
		<button class="menu-button" onclick={() => openForm('scheduled')}>
			{m.audit_usage_audit_logs_create_export_schedule()}
		</button>
	</DotDotDot>
{/snippet}

<svelte:head>
	<title>{pageTitle}</title>
</svelte:head>
