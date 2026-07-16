<script lang="ts">
	import { page } from '$app/state';
	import type { DateRange } from '$lib/components/Calendar.svelte';
	import Select from '$lib/components/Select.svelte';
	import { filterVisibleExportFields } from '$lib/components/admin/audit-log-exports/filterFields';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		Group,
		UserService,
		type AuditLogExport,
		type LLMAuditLogURLFilters,
		type OrgUser,
		type AuditLogURLFilters
	} from '$lib/services';
	import { profile } from '$lib/stores';
	import { TriangleAlert, ChevronDown, ChevronUp } from '@lucide/svelte';
	import { subDays, set } from 'date-fns';
	import { onMount } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type AuditLogExportMultiSelectFilterKey =
		| 'user_id'
		| 'mcp_id'
		| 'mcp_server_display_name'
		| 'mcp_server_catalog_entry_name'
		| 'call_type'
		| 'call_identifier'
		| 'client_name'
		| 'client_version'
		| 'client_ip'
		| 'response_status'
		| 'session_id'
		| 'agent_provider'
		| 'status'
		| 'tool_name'
		| 'tool_kind'
		| 'device_id';
	type LLMAuditLogExportMultiSelectFilterKey =
		| 'user_id'
		| 'client'
		| 'client_session_id'
		| 'message_policy_triggered'
		| 'model_provider'
		| 'outcome'
		| 'request_path'
		| 'response_status'
		| 'target_model';

	type AuditLogExportFilterFieldConfig<T extends string = string> = {
		filterKey: T;
		title: string;
		description: string;
		getOptionLabel?: (value: string) => string;
		useUserDisplayNames?: boolean;
	};

	const AUDIT_LOG_EXPORT_FILTER_FIELDS: AuditLogExportFilterFieldConfig<AuditLogExportMultiSelectFilterKey>[] =
		[
			{
				filterKey: 'user_id',
				title: 'Users',
				description: 'List of users',
				useUserDisplayNames: true
			},
			{
				filterKey: 'mcp_id',
				title: 'Server IDs',
				description: 'List of server IDs'
			},
			{
				filterKey: 'mcp_server_display_name',
				title: 'Server Names',
				description: 'List of server display names'
			},
			{
				filterKey: 'call_type',
				title: 'Call Types',
				description: 'List of call types'
			},
			{
				filterKey: 'client_name',
				title: 'Client Names',
				description: 'List of client names'
			},
			{
				filterKey: 'response_status',
				title: 'Response Status',
				description: 'List of HTTP status codes'
			},
			{
				filterKey: 'session_id',
				title: 'Session IDs',
				description: 'List of session IDs'
			},
			{
				filterKey: 'client_ip',
				title: 'Client IPs',
				description: 'List of IP addresses'
			},
			{
				filterKey: 'call_identifier',
				title: 'Call Identifier',
				description: 'List of call identifiers'
			},
			{
				filterKey: 'client_version',
				title: 'Client Versions',
				description: 'List of client versions'
			},
			{
				filterKey: 'mcp_server_catalog_entry_name',
				title: 'Catalog Entry Names',
				description: 'List of catalog entry names'
			},
			{
				filterKey: 'agent_provider',
				title: 'Agent Providers',
				description: 'List of local-agent providers'
			},
			{
				filterKey: 'status',
				title: 'Reported Statuses',
				description: 'List of local-agent statuses'
			},
			{
				filterKey: 'tool_name',
				title: 'Tool Names',
				description: 'List of local tool names'
			},
			{
				filterKey: 'tool_kind',
				title: 'Tool Kinds',
				description: 'List of local tool kinds'
			},
			{
				filterKey: 'device_id',
				title: 'Device IDs',
				description: 'List of enrolled device IDs'
			}
		];
	const LLM_AUDIT_LOG_EXPORT_FILTER_FIELDS: AuditLogExportFilterFieldConfig<LLMAuditLogExportMultiSelectFilterKey>[] =
		[
			{
				filterKey: 'user_id',
				title: 'Users',
				description: 'List of users',
				useUserDisplayNames: true
			},
			{
				filterKey: 'model_provider',
				title: 'Model Providers',
				description: 'List of model providers'
			},
			{
				filterKey: 'target_model',
				title: 'Target Models',
				description: 'List of target models'
			},
			{
				filterKey: 'request_path',
				title: 'Request Paths',
				description: 'List of request paths'
			},
			{
				filterKey: 'response_status',
				title: 'Response Status',
				description: 'List of HTTP status codes'
			},
			{
				filterKey: 'outcome',
				title: 'Outcomes',
				description: 'List of outcomes'
			},
			{
				filterKey: 'client',
				title: 'Clients',
				description: 'List of clients'
			},
			{
				filterKey: 'client_session_id',
				title: 'Client Session IDs',
				description: 'List of client session IDs'
			},
			{
				filterKey: 'message_policy_triggered',
				title: 'Message Policy Action',
				description: 'Filter by whether a message policy was triggered',
				getOptionLabel: (value) => (value === 'true' ? 'Triggered' : 'Not triggered')
			}
		];

	interface Props {
		onCancel: () => void;
		onSubmit: (result?: AuditLogExport) => void;
		mode?: 'create' | 'view' | 'edit';
		initialData?: AuditLogExport;
		logType?: 'mcp' | 'llm';
	}

	let { onCancel, onSubmit, mode = 'create', initialData, logType = 'mcp' }: Props = $props();

	let showAdvancedOptions = $state(false);
	let isViewMode = $derived(mode === 'view');
	let defaultKeyPrefix = $derived(logType === 'llm' ? 'llm-audit-logs' : 'mcp-audit-logs');

	const hasAuditorPermissions = $derived(profile.current.groups.includes(Group.AUDITOR));

	// The concrete audit-log sources to export. The API carries an explicit list of source types;
	// selecting more than one exports each source in the same export.
	const ALL_SOURCE_TYPES = ['mcp', 'local_agent_tool_call'] as const;
	const sourceTypeLabels: Record<string, string> = {
		mcp: 'MCP',
		local_agent_tool_call: 'Local Agent Tool Calls'
	};

	// Form state
	let form = $state({
		name: '',
		bucket: '',
		keyPrefix: '',
		startTime: subDays(new Date(), 7),
		endTime: set(new Date(), { milliseconds: 0, seconds: 59 }),
		sourceTypes: ['mcp'],
		filters: {
			user_id: '',
			mcp_id: '',
			mcp_server_display_name: '',
			mcp_server_catalog_entry_name: '',
			call_type: '',
			call_identifier: '',
			client_name: '',
			client_version: '',
			client_ip: '',
			response_status: '',
			session_id: '',
			client: '',
			client_session_id: '',
			message_policy_triggered: '',
			model_provider: '',
			outcome: '',
			request_path: '',
			target_model: '',
			agent_provider: '',
			status: '',
			tool_name: '',
			tool_kind: '',
			device_id: '',
			query: ''
		} as Partial<AuditLogURLFilters & LLMAuditLogURLFilters>
	});

	let creating = $state(false);
	let error = $state('');

	let mcpFiltersIds = [
		'mcp_id',
		'user_id',
		'mcp_server_catalog_entry_name',
		'mcp_server_display_name',
		'call_identifier',
		'client_name',
		'client_version',
		'client_ip',
		'call_type',
		'session_id',
		'response_status',
		'agent_provider',
		'status',
		'tool_name',
		'tool_kind',
		'device_id'
	];
	let llmFiltersIds = [
		'user_id',
		'client',
		'client_session_id',
		'message_policy_triggered',
		'model_provider',
		'outcome',
		'request_path',
		'response_status',
		'target_model'
	];
	let filtersIds = $derived(logType === 'llm' ? llmFiltersIds : mcpFiltersIds);

	let usersMap = new SvelteMap<string, OrgUser>();
	let filtersOptions: Record<string, string[]> = $state({});

	onMount(async () => {
		if (initialData && (mode === 'view' || mode === 'edit')) {
			form.name = initialData.name || '';
			form.bucket = initialData.bucket || '';
			form.keyPrefix = initialData.keyPrefix || '';
			form.startTime = initialData.startTime ? new Date(initialData.startTime) : form.startTime;
			form.endTime = initialData.endTime ? new Date(initialData.endTime) : form.endTime;

			if (logType === 'llm' && initialData.llmFilters) {
				const filters = initialData.llmFilters;
				form.filters = {
					user_id: join(filters.userIDs),
					model_provider: join(filters.modelProviders),
					target_model: join(filters.targetModels),
					request_path: join(filters.requestPaths),
					response_status: join(filters.responseStatuses?.map(String)),
					outcome: join(filters.outcomes),
					client: join(filters.clients),
					client_session_id: join(filters.clientSessionIDs),
					message_policy_triggered: join(filters.messagePolicyTriggered?.map(String)),
					query: filters.query ?? ''
				};
				showAdvancedOptions = true;
				return;
			}

			if (initialData.filters) {
				const filters = initialData.filters;
				form.sourceTypes = normalizeSourceTypes(filters.sourceTypes);
				form.filters = {
					user_id: join(filters.userIDs),
					mcp_id: join(filters.mcpIDs),
					mcp_server_display_name: join(filters.mcpServerDisplayNames),
					mcp_server_catalog_entry_name: join(filters.mcpServerCatalogEntryNames),
					call_type: join(filters.callTypes),
					call_identifier: join(filters.callIdentifiers),
					response_status: join(filters.responseStatuses),
					session_id: join(filters.sessionIDs),
					client_name: join(filters.clientNames),
					client_version: join(filters.clientVersions),
					client_ip: join(filters.clientIPs),
					agent_provider: join(filters.agentProviders),
					status: join(filters.statuses),
					tool_name: join(filters.toolNames),
					tool_kind: join(filters.toolKinds),
					device_id: join(filters.deviceIDs),
					query: filters.query ?? ''
				};
				showAdvancedOptions = true;
			}
		} else if (mode === 'create') {
			// Populate from URL parameters for create mode
			const params = page.url.searchParams;

			// Set time range if provided
			const startTime = params.get('startTime') ?? params.get('start_time');
			const endTime = params.get('endTime') ?? params.get('end_time');
			if (startTime) {
				form.startTime = new Date(startTime);
			}
			if (endTime) {
				form.endTime = new Date(endTime);
			}

			if (logType === 'mcp') {
				const eventTypes = params.get('event_type')?.split(',') ?? [];
				form.sourceTypes = normalizeSourceTypes(
					eventTypes.map((eventType) =>
						eventType === 'local_agent_tool_call' ? 'local_agent_tool_call' : 'mcp'
					)
				);
			}

			// Set filters if provided
			const filterKeys = logType === 'llm' ? llmFiltersIds : mcpFiltersIds;

			let hasFilters = false;
			filterKeys.forEach((key) => {
				const value = params.get(key);
				if (value && key in form.filters) {
					(form.filters as Record<string, string>)[key] = value;
					hasFilters = true;
				}
			});
			const query = params.get('query');
			if (query) {
				form.filters.query = query;
				hasFilters = true;
			}

			// Show advanced options if there are filters from the URL
			if (hasFilters) {
				showAdvancedOptions = true;
			}
		}
	});

	$effect(() => {
		UserService.listUsers().then((res) => {
			res.forEach((user) => {
				usersMap.set(user.id, user);
			});
		});
	});

	$effect(() => {
		const event_type = form.sourceTypes
			.map((source) => (source === 'mcp' ? 'mcp_call' : 'local_agent_tool_call'))
			.join(',');
		filtersIds.forEach((id) => {
			if (logType === 'llm') {
				AdminService.listLLMAuditLogFilterOptions(id).then((res) => {
					filtersOptions[id] = res.options ?? [];
				});
				return;
			}
			if (
				localFilterKeys.has(id as AuditLogExportMultiSelectFilterKey) &&
				!form.sourceTypes.includes('local_agent_tool_call')
			)
				return;
			if (
				mcpFilterKeys.has(id as AuditLogExportMultiSelectFilterKey) &&
				!form.sourceTypes.includes('mcp')
			)
				return;
			UserService.listAuditLogFilterOptions(id, { event_type }).then((res) => {
				filtersOptions[id] = res.options ?? [];
			});
		});
	});

	const mcpFilterKeys = new Set<AuditLogExportMultiSelectFilterKey>([
		'mcp_id',
		'mcp_server_display_name',
		'mcp_server_catalog_entry_name',
		'call_type',
		'call_identifier',
		'client_name',
		'client_version',
		'response_status'
	]);
	const localFilterKeys = new Set<AuditLogExportMultiSelectFilterKey>([
		'agent_provider',
		'status',
		'tool_name',
		'tool_kind',
		'device_id'
	]);
	const visibleAuditLogExportFields = $derived(
		filterVisibleExportFields(form, AUDIT_LOG_EXPORT_FILTER_FIELDS, mcpFilterKeys, localFilterKeys)
	);

	function join(array: string[] | undefined): string {
		return array ? array.join(',') : '';
	}

	function split(value: string | null | undefined): string[] {
		return value
			? value
					.split(',')
					.map((s) => s.trim())
					.filter((s) => s.length > 0)
			: [];
	}

	function splitNumbers(value: string | null | undefined): number[] {
		return split(value)
			.map((s) => Number(s))
			.filter((n) => !Number.isNaN(n));
	}

	// Keep only known source types, preserving their canonical order, and fall back to the
	// historical MCP-only default when nothing valid is selected.
	function normalizeSourceTypes(sourceTypes: string[] | undefined): string[] {
		const selected = ALL_SOURCE_TYPES.filter((st) => sourceTypes?.includes(st));
		return selected.length > 0 ? [...selected] : ['mcp'];
	}

	function toggleSourceType(sourceType: string, checked: boolean) {
		// normalizeSourceTypes de-duplicates, so appending on check is safe.
		const next = checked
			? [...form.sourceTypes, sourceType]
			: form.sourceTypes.filter((st) => st !== sourceType);
		form.sourceTypes = normalizeSourceTypes(next);
	}

	function splitBooleans(value: string | null | undefined): boolean[] {
		return split(value).flatMap((item) =>
			item === 'true' ? [true] : item === 'false' ? [false] : []
		);
	}

	async function handleSubmit() {
		try {
			creating = true;
			error = '';

			// Validate required fields
			if (!form.name) {
				throw new Error('Name is required');
			}
			if (!form.bucket) {
				throw new Error('Bucket name is required');
			}

			if (logType === 'llm') {
				const request = {
					name: form.name,
					type: 'llm' as const,
					bucket: form.bucket,
					keyPrefix: form.keyPrefix,
					startTime: form.startTime.toISOString(),
					endTime: form.endTime.toISOString(),
					llmFilters: {
						userIDs: split(form.filters.user_id),
						modelProviders: split(form.filters.model_provider),
						targetModels: split(form.filters.target_model),
						requestPaths: split(form.filters.request_path),
						responseStatuses: splitNumbers(form.filters.response_status),
						outcomes: split(form.filters.outcome),
						clients: split(form.filters.client),
						clientSessionIDs: split(form.filters.client_session_id),
						messagePolicyTriggered: splitBooleans(form.filters.message_policy_triggered),
						query: form.filters.query ?? ''
					}
				};

				const result = (await AdminService.createAuditLogExport(request)) as AuditLogExport;
				onSubmit(result);
				return;
			}

			// Prepare the request
			const request = {
				name: form.name,
				type: 'mcp' as const,
				bucket: form.bucket,
				keyPrefix: form.keyPrefix,
				startTime: form.startTime.toISOString(),
				endTime: form.endTime.toISOString(),
				filters: {
					sourceTypes: normalizeSourceTypes(form.sourceTypes),
					userIDs: split(form.filters.user_id),
					mcpIDs: split(form.filters.mcp_id),
					mcpServerDisplayNames: split(form.filters.mcp_server_display_name),
					mcpServerCatalogEntryNames: split(form.filters.mcp_server_catalog_entry_name),
					callTypes: split(form.filters.call_type),
					callIdentifiers: split(form.filters.call_identifier),
					responseStatuses: split(form.filters.response_status),
					sessionIDs: split(form.filters.session_id),
					clientNames: split(form.filters.client_name),
					clientVersions: split(form.filters.client_version),
					clientIPs: split(form.filters.client_ip),
					agentProviders: split(form.filters.agent_provider),
					statuses: split(form.filters.status),
					toolNames: split(form.filters.tool_name),
					toolKinds: split(form.filters.tool_kind),
					deviceIDs: split(form.filters.device_id),
					query: form.filters.query ?? ''
				}
			};

			const result = (await AdminService.createAuditLogExport(request)) as AuditLogExport;

			onSubmit(result);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to create export';
		} finally {
			creating = false;
		}
	}

	function handleDateChange({ start, end }: DateRange) {
		if (start) {
			form.startTime = start;
		}
		if (end) {
			form.endTime = end;
		}
	}

	function selectOptionsForField(
		field: AuditLogExportFilterFieldConfig
	): { id: string; label: string }[] {
		const opts = filtersOptions[field.filterKey];
		if (!opts?.map) return [];
		if (field.useUserDisplayNames) {
			return opts.map((d) => ({ id: d, label: usersMap.get(d)?.displayName ?? d }));
		}
		return opts.map((d) => ({ id: d, label: field.getOptionLabel?.(d) ?? d }));
	}
</script>

<div class="paper">
	<form
		class="space-y-8"
		onsubmit={(e) => {
			e.preventDefault();
			handleSubmit();
		}}
	>
		{#if !hasAuditorPermissions}
			<div class="flex items-start gap-3 rounded-md border border-warning bg-warning/10 p-4">
				<TriangleAlert class="size-5 text-warning" />
				<div class="text-sm">
					Exported logs will not include request/response headers and body information. Auditor role
					is required to access this data.
				</div>
			</div>
		{/if}
		<!-- Basic Information -->
		<div class="flex flex-col gap-4">
			<h3 class="text-lg font-semibold">
				{#if mode === 'view'}
					Export Details
				{:else if mode === 'edit'}
					Edit Export
				{:else}
					Basic Information
				{/if}
			</h3>
			<div class="grid grid-cols-1 justify-between gap-6 lg:grid-cols-2">
				<div class="flex flex-col gap-1">
					<label class="text-sm font-medium" for="name">Export Name</label>
					<input
						class={twMerge(
							'text-input-filled',
							isViewMode && 'text-[currentColor] disabled:opacity-100'
						)}
						id="name"
						bind:value={form.name}
						placeholder="audit-export-2024"
						required={!isViewMode}
						readonly={isViewMode}
						disabled={isViewMode}
					/>
					{#if (isViewMode && form.name) || !isViewMode}
						<p class="text-muted-content text-xs">Unique name for this export</p>
					{/if}
				</div>
				<div class="flex flex-col gap-1">
					<label class="text-sm font-medium" for="bucket">Bucket Name</label>
					<input
						class={twMerge(
							'text-input-filled',
							isViewMode && 'text-[currentColor] disabled:opacity-100'
						)}
						id="bucket"
						bind:value={form.bucket}
						placeholder="my-audit-exports"
						required={!isViewMode}
						readonly={isViewMode}
						disabled={isViewMode}
					/>
					{#if (isViewMode && form.bucket) || !isViewMode}
						<p class="text-muted-content text-xs">
							Storage bucket name where exports will be saved
						</p>
					{/if}
				</div>
			</div>

			<div class="flex flex-col gap-1">
				<label class="text-sm font-medium" for="keyPrefix">Key Prefix (Optional)</label>
				<input
					class={twMerge(
						'text-input-filled',
						isViewMode && 'text-[currentColor] disabled:opacity-100'
					)}
					id="keyPrefix"
					bind:value={form.keyPrefix}
					placeholder={`Leave empty for default: ${defaultKeyPrefix}/YYYY/MM/DD/`}
					readonly={isViewMode}
					disabled={isViewMode}
				/>
				{#if (isViewMode && form.keyPrefix) || !isViewMode}
					<p class="text-muted-content text-xs">
						Path prefix within the bucket. If empty, defaults to "{defaultKeyPrefix}/YYYY/MM/DD/"
						format based on current date.
					</p>
				{/if}
			</div>

			<div class="flex flex-col gap-1">
				<label class="text-sm font-medium" for="timeRange">Time Range</label>
				<AuditLogCalendar
					start={form.startTime}
					end={form.endTime}
					onChange={mode === 'view' ? () => {} : handleDateChange}
					disabled={isViewMode}
				/>
			</div>

			{#if logType === 'mcp'}
				<div class="flex flex-col gap-1">
					<span class="text-sm font-medium">Log Sources</span>
					<div class="flex flex-col gap-2 py-1">
						{#each ALL_SOURCE_TYPES as sourceType (sourceType)}
							<label class="flex items-center gap-2 text-sm">
								<input
									type="checkbox"
									checked={form.sourceTypes.includes(sourceType)}
									disabled={isViewMode}
									onchange={(e) => toggleSourceType(sourceType, e.currentTarget.checked)}
								/>
								{sourceTypeLabels[sourceType]}
							</label>
						{/each}
					</div>
					{#if !isViewMode}
						<p class="text-muted-content text-xs">
							Which audit-log source(s) to export. Select both to include MCP and local-agent
							tool-call logs in the same export.
						</p>
					{/if}
				</div>
			{/if}
		</div>

		<!-- Advanced Options -->
		<div class="space-y-4">
			<button
				type="button"
				class="flex w-full items-center justify-between text-left"
				onclick={() => {
					showAdvancedOptions = !showAdvancedOptions;
				}}
			>
				<h3 class="text-lg font-semibold">Advanced Options</h3>
				{#if showAdvancedOptions}
					<ChevronUp class="size-5" />
				{:else}
					<ChevronDown class="size-5" />
				{/if}
			</button>

			{#if showAdvancedOptions}
				<div transition:slide={{ duration: 200 }} class="space-y-4">
					<p class="text-sm text-gray-600">
						Leave filters empty to export all logs in the selected time range
					</p>

					<div class="flex flex-col gap-1">
						<label class="text-sm font-medium" for="query">Search Query</label>
						<input
							id="query"
							class={twMerge(
								'text-input-filled',
								isViewMode && 'text-[currentColor] disabled:opacity-100'
							)}
							bind:value={form.filters.query}
							placeholder="Search audit logs"
							readonly={isViewMode}
							disabled={isViewMode}
						/>
						<p class="text-muted-content text-xs">
							Free-text search to apply to the exported audit logs
						</p>
					</div>

					{#snippet auditLogExportFilterSelect(
						filterKey: AuditLogExportMultiSelectFilterKey | LLMAuditLogExportMultiSelectFilterKey,
						title: string,
						description: string,
						selectOptions: { id: string; label: string }[]
					)}
						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for={filterKey}>{title}</label>
							<Select
								id={filterKey}
								class={twMerge(
									'dark:border-base-400 bg-base-100 dark:bg-base-100 border border-transparent shadow-inner',
									isViewMode && 'text-[currentColor] disabled:opacity-100'
								)}
								classes={{
									root: 'w-full',
									clear: 'hover:bg-base-400 bg-transparent'
								}}
								options={selectOptions}
								bind:selected={
									() => form.filters[filterKey] ?? '', (v) => (form.filters[filterKey] = v ?? '')
								}
								disabled={isViewMode}
								readonly={isViewMode}
								multiple
							/>
							{#if (isViewMode && form.filters[filterKey]) || !isViewMode}
								<p class="text-muted-content text-xs">{description}</p>
							{/if}
						</div>
					{/snippet}

					<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
						{#each logType === 'llm' ? LLM_AUDIT_LOG_EXPORT_FILTER_FIELDS : visibleAuditLogExportFields as field (field.filterKey)}
							{@render auditLogExportFilterSelect(
								field.filterKey,
								field.title,
								field.description,
								selectOptionsForField(field)
							)}
						{/each}
					</div>
				</div>
			{/if}
		</div>

		<!-- Error Display -->
		{#if error}
			<div class="flex items-start gap-3 rounded-md bg-error/10 p-4">
				<TriangleAlert class="size-5 text-error" />
				<div class="text-sm text-error">
					{error}
				</div>
			</div>
		{/if}

		<!-- Actions -->
		<div class="flex justify-end gap-3 pt-6">
			<button
				type="button"
				class="btn btn-secondary"
				onclick={onCancel}
				disabled={creating && mode !== 'view'}
			>
				{mode === 'view' ? 'Back' : 'Cancel'}
			</button>
			{#if mode !== 'view'}
				<button type="submit" class="btn btn-primary" disabled={creating}>
					{#if creating}
						<Loading class="size-4" />
						{mode === 'edit' ? 'Saving Changes...' : 'Creating Export...'}
					{:else}
						{mode === 'edit' ? 'Save Changes' : 'Create Export'}
					{/if}
				</button>
			{/if}
		</div>
	</form>
</div>
