<script lang="ts">
	import { page } from '$app/state';
	import { toAuditLogFilterSelectOption, toStringFilterSelectOptions } from '$lib/auditlogs';
	import type { DateRange } from '$lib/components/Calendar.svelte';
	import Select from '$lib/components/Select.svelte';
	import {
		ALL_SOURCE_TYPES,
		clearSourceScopedFilters,
		COMMON_FILTER_KEYS,
		eventTypeParamFromSourceTypes,
		filterVisibleExportFields,
		isExportFilterKeyVisible,
		normalizeSourceTypes,
		sourceTypeLabels,
		sourceTypesFromEventTypeParam
	} from '$lib/components/admin/audit-log-exports/filterFields';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { parseMultiValue, serializeMultiValue } from '$lib/multiValue';
	import {
		AdminService,
		Group,
		UserService,
		type AuditLogExport,
		type AuditLogFilterOption,
		type LLMAuditLogURLFilters,
		type OrgUser,
		type AuditLogURLFilters
	} from '$lib/services';
	import { profile } from '$lib/stores';
	import { getUserDisplayName } from '$lib/utils';
	import { TriangleAlert, ChevronDown, ChevronUp } from '@lucide/svelte';
	import { subDays, set } from 'date-fns';
	import { onMount } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type AuditLogExportMultiSelectFilterKey =
		| 'api_key_id'
		| 'actor'
		| 'operation'
		| 'mcp_server'
		| 'tool'
		| 'outcome'
		| 'client'
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
		| 'api_key_id'
		| 'user_id'
		| 'user_agent'
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
			// Common cross-source filters. Shown only when more than one log source is selected.
			{
				filterKey: 'actor',
				title: m.audit_usage_exports_filter_title_actors(),
				description: m.audit_usage_exports_filter_desc_users_and_devices(),
				useUserDisplayNames: true
			},
			{
				filterKey: 'tool',
				title: m.audit_usage_exports_filter_title_tools(),
				description: m.audit_usage_exports_filter_desc_tools_called()
			},
			{
				filterKey: 'mcp_server',
				title: m.audit_usage_exports_filter_title_mcp_servers(),
				description: m.audit_usage_exports_filter_desc_mcp_servers_parent()
			},
			{
				filterKey: 'operation',
				title: m.audit_usage_exports_filter_title_operations(),
				description: m.audit_usage_exports_filter_desc_mcp_operations()
			},
			{
				filterKey: 'outcome',
				title: m.audit_usage_exports_filter_title_outcomes(),
				description: m.audit_usage_exports_filter_desc_outcome_values()
			},
			{
				filterKey: 'client',
				title: m.audit_usage_exports_filter_title_clients(),
				description: m.audit_usage_exports_filter_desc_mcp_clients_and_providers()
			},
			// API-key attribution is shared by every audit-log source.
			{
				filterKey: 'api_key_id',
				title: m.audit_usage_exports_filter_title_api_keys(),
				description: m.audit_usage_exports_filter_desc_api_keys_used()
			},
			// Single-source filters. Shown only when exactly one log source is selected.
			{
				filterKey: 'user_id',
				title: m.audit_usage_exports_filter_title_users(),
				description: m.audit_usage_exports_filter_desc_list_users(),
				useUserDisplayNames: true
			},
			{
				filterKey: 'mcp_id',
				title: m.audit_usage_exports_filter_title_server_ids(),
				description: m.audit_usage_exports_filter_desc_list_server_ids()
			},
			{
				filterKey: 'mcp_server_display_name',
				title: m.audit_usage_exports_filter_title_server_names(),
				description: m.audit_usage_exports_filter_desc_list_server_display_names()
			},
			{
				filterKey: 'call_type',
				title: m.audit_usage_exports_filter_title_call_types(),
				description: m.audit_usage_exports_filter_desc_list_call_types()
			},
			{
				filterKey: 'client_name',
				title: m.audit_usage_exports_filter_title_client_names(),
				description: m.audit_usage_exports_filter_desc_list_client_names()
			},
			{
				filterKey: 'response_status',
				title: m.audit_usage_exports_filter_title_response_status(),
				description: m.audit_usage_exports_filter_desc_list_http_status_codes()
			},
			{
				filterKey: 'session_id',
				title: m.audit_usage_exports_filter_title_session_ids(),
				description: m.audit_usage_exports_filter_desc_list_session_ids()
			},
			{
				filterKey: 'client_ip',
				title: m.audit_usage_exports_filter_title_client_ips(),
				description: m.audit_usage_exports_filter_desc_list_ip_addresses()
			},
			{
				filterKey: 'call_identifier',
				title: m.audit_usage_exports_filter_title_call_identifier(),
				description: m.audit_usage_exports_filter_desc_list_call_identifiers()
			},
			{
				filterKey: 'client_version',
				title: m.audit_usage_exports_filter_title_client_versions(),
				description: m.audit_usage_exports_filter_desc_list_client_versions()
			},
			{
				filterKey: 'mcp_server_catalog_entry_name',
				title: m.audit_usage_exports_filter_title_catalog_entry_names(),
				description: m.audit_usage_exports_filter_desc_list_catalog_entry_names()
			},
			{
				filterKey: 'agent_provider',
				title: m.audit_usage_exports_filter_title_agent_providers(),
				description: m.audit_usage_exports_filter_desc_list_local_agent_providers()
			},
			{
				filterKey: 'status',
				title: m.audit_usage_exports_filter_title_reported_statuses(),
				description: m.audit_usage_exports_filter_desc_list_local_agent_statuses()
			},
			{
				filterKey: 'tool_name',
				title: m.audit_usage_exports_filter_title_tool_names(),
				description: m.audit_usage_exports_filter_desc_list_local_tool_names()
			},
			{
				filterKey: 'tool_kind',
				title: m.audit_usage_exports_filter_title_tool_kinds(),
				description: m.audit_usage_exports_filter_desc_list_local_tool_kinds()
			},
			{
				filterKey: 'device_id',
				title: m.audit_usage_exports_filter_title_device_ids(),
				description: m.audit_usage_exports_filter_desc_list_enrolled_device_ids()
			}
		];
	const LLM_AUDIT_LOG_EXPORT_FILTER_FIELDS: AuditLogExportFilterFieldConfig<LLMAuditLogExportMultiSelectFilterKey>[] =
		[
			{
				filterKey: 'api_key_id',
				title: m.audit_usage_exports_filter_title_api_keys(),
				description: m.audit_usage_exports_filter_desc_api_keys_used()
			},
			{
				filterKey: 'user_id',
				title: m.audit_usage_exports_filter_title_users(),
				description: m.audit_usage_exports_filter_desc_list_users(),
				useUserDisplayNames: true
			},
			{
				filterKey: 'model_provider',
				title: m.audit_usage_exports_filter_title_model_providers(),
				description: m.audit_usage_exports_filter_desc_list_model_providers()
			},
			{
				filterKey: 'target_model',
				title: m.audit_usage_exports_filter_title_target_models(),
				description: m.audit_usage_exports_filter_desc_list_target_models()
			},
			{
				filterKey: 'request_path',
				title: m.audit_usage_exports_filter_title_request_paths(),
				description: m.audit_usage_exports_filter_desc_list_request_paths()
			},
			{
				filterKey: 'response_status',
				title: m.audit_usage_exports_filter_title_response_status(),
				description: m.audit_usage_exports_filter_desc_list_http_status_codes()
			},
			{
				filterKey: 'outcome',
				title: m.audit_usage_exports_filter_title_outcomes(),
				description: m.audit_usage_exports_filter_desc_list_outcomes()
			},
			{
				filterKey: 'user_agent',
				title: m.audit_usage_exports_filter_title_user_agents(),
				description: m.audit_usage_exports_filter_desc_list_user_agents()
			},
			{
				filterKey: 'client_session_id',
				title: m.audit_usage_exports_filter_title_client_session_ids(),
				description: m.audit_usage_exports_filter_desc_list_client_session_ids()
			},
			{
				filterKey: 'message_policy_triggered',
				title: m.audit_usage_exports_filter_title_message_policy_action(),
				description: m.audit_usage_exports_filter_desc_message_policy_triggered(),
				getOptionLabel: (value) =>
					value === 'true'
						? m.audit_usage_exports_triggered()
						: m.audit_usage_exports_not_triggered()
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

	// Form state. Every log source starts selected so a new export covers everything by default;
	// the user narrows it by unchecking.
	let form = $state({
		name: '',
		bucket: '',
		keyPrefix: '',
		startTime: subDays(new Date(), 7),
		endTime: set(new Date(), { milliseconds: 0, seconds: 59 }),
		sourceTypes: [...ALL_SOURCE_TYPES] as string[],
		filters: {
			api_key_id: '',
			actor: '',
			operation: '',
			mcp_server: '',
			tool: '',
			client: '',
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
			user_agent: '',
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
		'actor',
		'operation',
		'mcp_server',
		'tool',
		'outcome',
		'client',
		'api_key_id',
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
		'api_key_id',
		'user_id',
		'user_agent',
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
	let filtersOptions: Record<string, AuditLogFilterOption[]> = $state({});

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
					api_key_id: join(filters.apiKeyIDs),
					user_id: join(filters.userIDs),
					model_provider: join(filters.modelProviders),
					target_model: join(filters.targetModels),
					request_path: join(filters.requestPaths),
					response_status: join(filters.responseStatuses?.map(String)),
					outcome: join(filters.outcomes),
					user_agent: join(filters.userAgents),
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
					api_key_id: join(filters.apiKeyIDs),
					actor: join(filters.actors),
					operation: join(filters.operations),
					mcp_server: serializeMultiValue(filters.mcpServers ?? []),
					tool: join(filters.tools),
					outcome: join(filters.outcomes),
					client: join(filters.clients),
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
				// The audit-logs page omits event_type when no Source filter is applied; in that case
				// the all-sources default stands.
				const sourceTypes = sourceTypesFromEventTypeParam(params.get('event_type'));
				if (sourceTypes) {
					form.sourceTypes = sourceTypes;
				}
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
		UserService.listUsersIncludeDeleted().then((res) => {
			res.forEach((user) => {
				usersMap.set(user.id, user);
			});
		});
	});

	$effect(() => {
		const event_type = eventTypeParamFromSourceTypes(form.sourceTypes);
		filtersIds.forEach((id) => {
			if (logType === 'llm') {
				AdminService.listLLMAuditLogFilterOptions(id).then((res) => {
					filtersOptions[id] = res.options ?? [];
				});
				return;
			}
			// Only fetch options for the filters actually visible in the current source selection.
			if (
				!isExportFilterKeyVisible(
					form.sourceTypes,
					id,
					commonFilterKeys,
					mcpFilterKeys,
					localFilterKeys
				)
			)
				return;
			UserService.listAuditLogFilterOptions(id, { event_type }).then((res) => {
				filtersOptions[id] = res.options ?? [];
			});
		});
	});

	const commonFilterKeys = new Set<AuditLogExportMultiSelectFilterKey>(COMMON_FILTER_KEYS);
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
		filterVisibleExportFields(
			form,
			AUDIT_LOG_EXPORT_FILTER_FIELDS,
			commonFilterKeys,
			mcpFilterKeys,
			localFilterKeys
		)
	);

	function join(array: (string | number)[] | undefined): string {
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

	function toggleSourceType(sourceType: string, checked: boolean) {
		// normalizeSourceTypes de-duplicates, so appending on check is safe.
		const next = checked
			? [...form.sourceTypes, sourceType]
			: form.sourceTypes.filter((st) => st !== sourceType);
		form.sourceTypes = normalizeSourceTypes(next);
		clearSourceScopedFilters(form.filters);
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
				throw new Error(m.audit_usage_exports_name_required());
			}
			if (!form.bucket) {
				throw new Error(m.audit_usage_exports_bucket_required());
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
						apiKeyIDs: splitNumbers(form.filters.api_key_id),
						userIDs: split(form.filters.user_id),
						modelProviders: split(form.filters.model_provider),
						targetModels: split(form.filters.target_model),
						requestPaths: split(form.filters.request_path),
						responseStatuses: splitNumbers(form.filters.response_status),
						outcomes: split(form.filters.outcome),
						userAgents: split(form.filters.user_agent),
						clientSessionIDs: split(form.filters.client_session_id),
						messagePolicyTriggered: splitBooleans(form.filters.message_policy_triggered),
						query: form.filters.query ?? ''
					}
				};

				const result = (await AdminService.createAuditLogExport(request)) as AuditLogExport;
				onSubmit(result);
				return;
			}

			if (form.sourceTypes.length === 0) {
				throw new Error(m.audit_usage_exports_source_required());
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
					apiKeyIDs: splitNumbers(form.filters.api_key_id),
					sourceTypes: normalizeSourceTypes(form.sourceTypes),
					actors: split(form.filters.actor),
					operations: split(form.filters.operation),
					mcpServers: parseMultiValue(form.filters.mcp_server),
					tools: split(form.filters.tool),
					outcomes: split(form.filters.outcome),
					clients: split(form.filters.client),
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
			error = err instanceof Error ? err.message : m.audit_usage_exports_create_failed();
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
		const resolveUserDisplayName = (id: string) =>
			usersMap.has(id) ? getUserDisplayName(usersMap, id) : id;
		if (field.useUserDisplayNames) {
			return toStringFilterSelectOptions(opts, resolveUserDisplayName);
		}
		return opts.map((d) => {
			const option = toAuditLogFilterSelectOption(d, resolveUserDisplayName);
			return typeof d === 'string' && field.getOptionLabel
				? { ...option, label: field.getOptionLabel(d) }
				: option;
		});
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
					{m.audit_usage_exports_auditor_notice()}
				</div>
			</div>
		{/if}
		<!-- Basic Information -->
		<div class="flex flex-col gap-4">
			<h3 class="text-lg font-semibold">
				{#if mode === 'view'}
					{m.audit_usage_exports_details()}
				{:else if mode === 'edit'}
					{m.audit_usage_exports_edit()}
				{:else}
					{m.audit_usage_exports_basic_information()}
				{/if}
			</h3>
			<div class="grid grid-cols-1 justify-between gap-6 lg:grid-cols-2">
				<div class="flex flex-col gap-1">
					<label class="text-sm font-medium" for="name">{m.audit_usage_exports_name_label()}</label>
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
						<p class="text-muted-content text-xs">{m.audit_usage_exports_name_help()}</p>
					{/if}
				</div>
				<div class="flex flex-col gap-1">
					<label class="text-sm font-medium" for="bucket"
						>{m.audit_usage_exports_bucket_label()}</label
					>
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
							{m.audit_usage_exports_bucket_help()}
						</p>
					{/if}
				</div>
			</div>

			<div class="flex flex-col gap-1">
				<label class="text-sm font-medium" for="keyPrefix"
					>{m.audit_usage_exports_key_prefix_label()}</label
				>
				<input
					class={twMerge(
						'text-input-filled',
						isViewMode && 'text-[currentColor] disabled:opacity-100'
					)}
					id="keyPrefix"
					bind:value={form.keyPrefix}
					placeholder={m.audit_usage_exports_key_prefix_placeholder({ prefix: defaultKeyPrefix })}
					readonly={isViewMode}
					disabled={isViewMode}
				/>
				{#if (isViewMode && form.keyPrefix) || !isViewMode}
					<p class="text-muted-content text-xs">
						{m.audit_usage_exports_key_prefix_help({ prefix: defaultKeyPrefix })}
					</p>
				{/if}
			</div>

			<div class="flex flex-col gap-1">
				<label class="text-sm font-medium" for="timeRange"
					>{m.audit_usage_exports_time_range()}</label
				>
				<AuditLogCalendar
					start={form.startTime}
					end={form.endTime}
					onChange={mode === 'view' ? () => {} : handleDateChange}
					disabled={isViewMode}
				/>
			</div>

			{#if logType === 'mcp'}
				<div class="flex flex-col gap-1">
					<span class="text-sm font-medium">{m.audit_usage_exports_log_sources()}</span>
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
							{m.audit_usage_exports_log_sources_help()}
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
				<h3 class="text-lg font-semibold">{m.audit_usage_exports_advanced_options()}</h3>
				{#if showAdvancedOptions}
					<ChevronUp class="size-5" />
				{:else}
					<ChevronDown class="size-5" />
				{/if}
			</button>

			{#if showAdvancedOptions}
				<div transition:slide={{ duration: 200 }} class="space-y-4">
					<p class="text-sm text-gray-600">
						{m.audit_usage_exports_leave_filters_empty()}
					</p>

					<div class="flex flex-col gap-1">
						<label class="text-sm font-medium" for="query"
							>{m.audit_usage_exports_search_query()}</label
						>
						<input
							id="query"
							class={twMerge(
								'text-input-filled',
								isViewMode && 'text-[currentColor] disabled:opacity-100'
							)}
							bind:value={form.filters.query}
							placeholder={m.audit_usage_exports_search_placeholder()}
							readonly={isViewMode}
							disabled={isViewMode}
						/>
						<p class="text-muted-content text-xs">
							{m.audit_usage_exports_search_help()}
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
								valueFormat={filterKey === 'mcp_server' ? 'json' : 'comma-separated'}
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
				{mode === 'view' ? m.common_back() : m.common_cancel()}
			</button>
			{#if mode !== 'view'}
				<button type="submit" class="btn btn-primary" disabled={creating}>
					{#if creating}
						<Loading class="size-4" />
						{mode === 'edit'
							? m.audit_usage_exports_saving_changes()
							: m.audit_usage_exports_creating()}
					{:else}
						{mode === 'edit' ? m.core_save_changes() : m.audit_usage_audit_logs_create_export()}
					{/if}
				</button>
			{/if}
		</div>
	</form>
</div>
