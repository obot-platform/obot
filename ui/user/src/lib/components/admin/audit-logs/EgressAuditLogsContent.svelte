<script lang="ts">
	import { page } from '$app/state';
	import { columnResize } from '$lib/actions/resize';
	import { buildPillSearchParamFilters, buildSearchParamFiltersArray } from '$lib/auditlogs';
	import FilterPills from '$lib/components/FilterPills.svelte';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import Pagination from '$lib/components/table/Pagination.svelte';
	import { setVirtualPageData } from '$lib/components/ui/virtual-page/context';
	import { m } from '$lib/i18n';
	import {
		AdminService,
		Group,
		UserService,
		type EgressAuditLog,
		type OrgUser
	} from '$lib/services';
	import { profile, responsive } from '$lib/stores';
	import { goto } from '$lib/url';
	import FiltersDrawer from '../filters-drawer/FiltersDrawer.svelte';
	import AuditLogDetails from './AuditLogDetails.svelte';
	import AuditLogTableSkeleton from './AuditLogTableSkeleton.svelte';
	import EgressAuditLogsTable from './EgressAuditLogsTable.svelte';
	import { Captions, Funnel } from '@lucide/svelte';
	import { endOfDay, set, subDays } from 'date-fns';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	type Entry = EgressAuditLog;
	const auditor = $derived(profile.current.groups.includes(Group.AUDITOR));
	let entries = $state<Entry[]>([]);
	let total = $state(0);
	const pageSize = 50;
	let offset = $state(0);
	const pageIndex = $derived(Math.floor(offset / pageSize));
	const lastPageIndex = $derived(Math.max(0, Math.ceil(total / pageSize) - 1));
	let loading = $state(false);
	let error = $state('');
	let selected = $state<Entry>();
	let showFilters = $state(false);
	let rightSidebar = $state<HTMLDivElement>();
	let users = $state<Record<string, OrgUser>>({});
	let environments = $state<Record<string, string>>({});
	let generation = 0;
	let detailGeneration = 0;

	type EgressDrawerFilters = {
		user_id?: string | null;
		environment_id?: string | null;
		claude_account?: string | null;
		claude_session?: string | null;
		host?: string | null;
		outcome?: string | null;
	};
	const supportedFilters: (keyof EgressDrawerFilters)[] = [
		'user_id',
		'environment_id',
		'outcome',
		'claude_account',
		'claude_session',
		'host'
	];

	const egressOutcomes = ['running', 'complete', 'denied', 'error', 'interrupted'];

	const searchParamFiltersAsArray = $derived(
		buildSearchParamFiltersArray<EgressDrawerFilters>(supportedFilters)
	);
	const searchParamFilters = $derived.by<EgressDrawerFilters>(() => {
		return searchParamFiltersAsArray.reduce((acc, [key, value]) => {
			acc[key] = value;
			return acc;
		}, {} as EgressDrawerFilters);
	});
	const pillsSearchParamFilters = $derived(
		buildPillSearchParamFilters<EgressDrawerFilters>(searchParamFiltersAsArray)
	);
	const hasFilterPills = $derived(Object.keys(pillsSearchParamFilters).length > 0);
	let query = $derived(page.url.searchParams.get('query') ?? '');

	const timeRangeFilters = $derived.by(() => {
		const startParam = page.url.searchParams.get('start_time');
		const endParam = page.url.searchParams.get('end_time');
		const endTime = set(new Date(endParam || new Date()), { milliseconds: 999, seconds: 59 });
		const startTime = startParam
			? set(new Date(startParam), { milliseconds: 0, seconds: 0 })
			: set(subDays(endTime, 7), { milliseconds: 0, seconds: 0 });
		return { startTime, endTime };
	});

	const filterPaginationKey = $derived(
		JSON.stringify({
			...pillsSearchParamFilters,
			query,
			start_time: timeRangeFilters.startTime.toISOString(),
			end_time: timeRangeFilters.endTime.toISOString()
		})
	);

	function appliedFilter(value: string | null | undefined) {
		return value?.trim() || undefined;
	}

	$effect(() => {
		void filterPaginationKey;
		offset = 0;
	});

	$effect(() => {
		const current = ++generation;
		const request = {
			limit: pageSize,
			offset,
			userId: appliedFilter(searchParamFilters.user_id),
			environmentId: appliedFilter(searchParamFilters.environment_id),
			claudeAccount: appliedFilter(searchParamFilters.claude_account),
			claudeSession: appliedFilter(searchParamFilters.claude_session),
			host: appliedFilter(searchParamFilters.host),
			outcome: appliedFilter(searchParamFilters.outcome),
			startTime: timeRangeFilters.startTime.toISOString(),
			endTime: timeRangeFilters.endTime.toISOString()
		};
		loading = true;
		error = '';
		void AdminService.listEgressAuditLogs(request)
			.then((response) => {
				if (current !== generation) return;
				entries = response.items ?? [];
				total = response.total;
				const lastOffset = Math.max(0, Math.ceil(response.total / pageSize) - 1) * pageSize;
				if (request.offset > lastOffset) offset = lastOffset;
			})
			.catch((e) => {
				if (current === generation) error = e instanceof Error ? e.message : String(e);
			})
			.finally(() => {
				if (current === generation) loading = false;
			});

		return () => {
			generation++;
		};
	});

	async function inspect(entry: Entry) {
		const current = ++detailGeneration;
		showFilters = false;
		selected = undefined;
		rightSidebar?.showPopover();
		try {
			const detail = await AdminService.getEgressAuditLog(entry.id);
			if (current === detailGeneration) selected = detail;
		} catch (e) {
			if (current === detailGeneration) {
				error = e instanceof Error ? e.message : String(e);
				rightSidebar?.hidePopover();
			}
		}
	}

	function decode(body?: string) {
		if (body === undefined || body === null) return undefined;
		const bytes = Uint8Array.from(atob(body), (c) => c.charCodeAt(0));
		try {
			const text = new TextDecoder('utf-8', { fatal: true }).decode(bytes);
			try {
				return JSON.parse(text);
			} catch {
				return text;
			}
		} catch {
			return { encoding: 'base64', content: body };
		}
	}

	function userName(id: string) {
		return users[id]?.displayName || users[id]?.email || id;
	}

	$effect(() => {
		setVirtualPageData(entries);
	});

	onMount(() => {
		void UserService.listUsersIncludeDeleted()
			.then((directory) => {
				users = Object.fromEntries(directory.map((u) => [u.id, u]));
			})
			.catch(() => {});
		void AdminService.listEgressAuditLogEnvironments()
			.then((data) => {
				environments = Object.fromEntries(
					Object.entries(data).sort((a, b) => a[1].localeCompare(b[1]))
				);
			})
			.catch(() => {});
		return () => {
			detailGeneration++;
		};
	});

	function handleDateChange({ start, end }: { start?: Date; end?: Date }) {
		const url = page.url;
		if (start) {
			url.searchParams.set('start_time', start.toISOString());
			url.searchParams.set('end_time', (end ?? endOfDay(start)).toISOString());
		}
		goto(url, { noScroll: true });
	}

	function getFilterDisplayLabel(key: string) {
		switch (key) {
			case 'user_id':
				return m.audit_usage_egress_user();
			case 'environment_id':
				return m.audit_usage_egress_environment();
			case 'claude_account':
				return m.audit_usage_egress_account();
			case 'claude_session':
				return m.audit_usage_egress_session();
			case 'host':
				return m.audit_usage_egress_destination();
			case 'outcome':
				return m.audit_usage_egress_outcome();
			default:
				return key;
		}
	}

	function getFilterValue(key: keyof EgressDrawerFilters, value: string | number) {
		const text = String(value);
		if (key === 'user_id') return userName(text);
		if (key === 'environment_id') return environments[text] || text;
		if (key === 'outcome') return outcomeLabel(text);
		return text;
	}

	function handleRightSidebarClose() {
		rightSidebar?.hidePopover();
		selected = undefined;
		showFilters = false;
		detailGeneration++;
	}

	function egressFilterOptions(filterId: string) {
		switch (filterId) {
			case 'user_id':
				return Object.keys(users);
			case 'environment_id':
				return Object.keys(environments);
			case 'outcome':
				return egressOutcomes;
			default:
				return [];
		}
	}

	function outcomeLabel(value: string) {
		switch (value) {
			case 'running':
				return m.audit_usage_egress_outcome_running();
			case 'complete':
				return m.audit_usage_egress_outcome_complete();
			case 'denied':
				return m.audit_usage_egress_outcome_denied();
			case 'error':
				return m.audit_usage_egress_outcome_error();
			case 'interrupted':
				return m.audit_usage_egress_outcome_interrupted();
			default:
				return value;
		}
	}
</script>

<div class="flex flex-col gap-4 pb-8">
	<div class="flex flex-col gap-2 @container">
		<div class="flex flex-col gap-4 justify-between @min-[768px]:flex-row">
			<div></div>

			<div class="flex gap-4 self-start @min-[768px]:self-end">
				<AuditLogCalendar
					start={timeRangeFilters.startTime}
					end={timeRangeFilters.endTime}
					onChange={handleDateChange}
				/>
				<button
					class="btn btn-neutral h-12.5"
					onclick={() => {
						showFilters = true;
						selected = undefined;
						detailGeneration++;
						rightSidebar?.showPopover();
					}}
				>
					<Funnel class="size-4" />
					{m.core_filters_title()}
				</button>
			</div>
		</div>
		{#if hasFilterPills}
			<FilterPills
				{pillsSearchParamFilters}
				{getFilterDisplayLabel}
				{getFilterValue}
				isMultiValueFilter={(key) => !['claude_account', 'claude_session', 'host'].includes(key)}
			/>
		{/if}
	</div>

	{#if error}
		<p role="alert" class="text-error text-sm">{error}</p>
	{/if}
	{#if loading}
		<AuditLogTableSkeleton />
	{:else if entries.length > 0}
		<EgressAuditLogsTable
			onSelectRow={(entry) => void inspect(entry)}
			getUserDisplayName={userName}
			getEnvironmentName={(id) => environments[id] || id}
			{outcomeLabel}
		/>
	{:else}
		<div class="flex flex-col items-center justify-center gap-4 px-6 py-16 text-center w-full">
			<Captions class="text-muted-content size-20 opacity-50" />
			<h4 class="text-muted-content text-lg font-semibold">{m.audit_usage_egress_empty()}</h4>
			<p class="text-muted-content max-w-md text-sm font-light">
				{m.audit_usage_audit_logs_no_logs_hint()}
			</p>
		</div>
	{/if}
	{#if total > pageSize}
		<Pagination
			{pageIndex}
			{lastPageIndex}
			{total}
			{loading}
			onPageChange={(idx) => {
				offset = idx * pageSize;
			}}
		/>
	{/if}
</div>

<div
	bind:this={rightSidebar}
	popover
	id="egress-audit-logs-details-sidebar"
	aria-label={selected ? m.audit_usage_egress_detail_title() : m.core_filters_title()}
	class={twMerge(
		'drawer-legacy',
		selected ? 'min-w-full max-w-full md:min-w-lg md:max-w-[85vw]' : 'md:w-lg lg:w-xl'
	)}
	style={selected && !responsive.isMobile ? 'width: 32rem' : ''}
	onbeforetoggle={(event) => {
		if (event.newState === 'closed') {
			selected = undefined;
			showFilters = false;
			detailGeneration++;
		}
	}}
>
	{#if selected && !responsive.isMobile && rightSidebar}
		<div
			role="none"
			class="absolute top-0 left-0 z-30 h-full w-3 cursor-col-resize"
			use:columnResize={{ column: rightSidebar, direction: 'right' }}
		></div>
	{/if}
	{#if selected}
		<AuditLogDetails
			auditLog={{
				...selected,
				user: userName(selected.userID),
				requestID: selected.id,
				requestHeaders: auditor ? selected.requestHeaders : undefined,
				responseHeaders: auditor ? selected.responseHeaders : undefined,
				requestBody: auditor ? decode(selected.requestBody) : undefined,
				responseBody: auditor ? decode(selected.responseBody) : undefined
			}}
			shouldShowPayload
			onClose={handleRightSidebarClose}
		>
			{#snippet preRequestContent(entry)}
				<div class="space-y-4 p-4 text-sm">
					<p class="break-all font-mono alert alert-info alert-soft">
						{entry.method}
						{entry.scheme}://{entry.host}{entry.path}
					</p>
					<div class="space-y-1 break-all">
						<p class="break-all grid grid-cols-2 gap-2">
							<span class="font-medium">{m.audit_usage_egress_account()}</span>
							{entry.claudeAccount}
						</p>
						<p class="break-all grid grid-cols-2 gap-2">
							<span class="font-medium">{m.audit_usage_egress_environment()}</span>
							{environments[entry.environmentID] || entry.environmentID}
						</p>
						<p class="break-all grid grid-cols-2 gap-2">
							<span class="font-medium">{m.audit_usage_egress_session()}</span>
							{entry.claudeSession}
						</p>
						<p class="break-all grid grid-cols-2 gap-2">
							<span class="font-medium">{m.audit_usage_egress_request()}</span>
							{m.audit_usage_egress_byte_count({ count: entry.requestBytes })}
						</p>
						<p class="break-all grid grid-cols-2 gap-2">
							<span class="font-medium">{m.audit_usage_egress_response()}</span>
							{m.audit_usage_egress_byte_count({ count: entry.responseBytes })}
						</p>
					</div>

					{#if entry.requestTruncated || entry.responseTruncated}
						<p class="alert alert-warning text-xs py-2 alert-soft">
							{entry.requestTruncated && entry.responseTruncated
								? m.audit_usage_egress_truncated_both()
								: entry.requestTruncated
									? m.audit_usage_egress_truncated_request()
									: m.audit_usage_egress_truncated_response()}
						</p>
					{/if}
				</div>
			{/snippet}
		</AuditLogDetails>
	{:else if showFilters}
		<FiltersDrawer
			onClose={handleRightSidebarClose}
			filters={searchParamFilters}
			getVisibleFilterKeys={() => supportedFilters}
			isFilterMultiSelect={() => false}
			isFilterTextInput={(key) => ['claude_account', 'claude_session', 'host'].includes(key)}
			getUserDisplayName={(id) => userName(id)}
			{getFilterDisplayLabel}
			getFilterOptionLabel={(key, value) => getFilterValue(key as keyof EgressDrawerFilters, value)}
			endpoint={async (filterId) => ({
				options: egressFilterOptions(filterId)
			})}
		/>
	{/if}
</div>
