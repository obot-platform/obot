<script lang="ts">
	import { page } from '$app/state';
	import { toAuditLogFilterSelectOption, toStringFilterSelectOptions } from '$lib/auditlogs';
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
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { parseMultiValue, serializeMultiValue } from '$lib/multiValue';
	import {
		type LLMAuditLogURLFilters,
		type AuditLogFilterOption,
		type OrgUser,
		type ScheduledAuditLogExport,
		AdminService,
		Group,
		UserService,
		type AuditLogURLFilters
	} from '$lib/services';
	import { profile } from '$lib/stores';
	import { getUserDisplayName } from '$lib/utils';
	import { TriangleAlert, GlobeIcon, ChevronDown, ChevronUp } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onCancel: () => void;
		onSubmit: (result?: ScheduledAuditLogExport) => void;
		mode?: 'create' | 'view' | 'edit';
		initialData?: ScheduledAuditLogExport;
		logType?: 'mcp' | 'llm';
	}

	let { onCancel, onSubmit, mode = 'create', initialData, logType = 'mcp' }: Props = $props();

	let defaultTimezone = $state(Intl.DateTimeFormat().resolvedOptions().timeZone);
	let showAdvancedOptions = $state(false);
	let isViewMode = $derived(mode === 'view');
	let defaultKeyPrefix = $derived(logType === 'llm' ? 'llm-audit-logs' : 'mcp-audit-logs');

	// Form state. Every log source starts selected so a new schedule covers everything by default;
	// the user narrows it by unchecking.
	let form = $state({
		name: '',
		enabled: true,
		bucket: '',
		keyPrefix: '',
		schedule: {
			interval: 'daily',
			hour: 3,
			minute: 0,
			day: 0,
			weekday: 1,
			timezone: Intl.DateTimeFormat().resolvedOptions().timeZone
		},
		retentionPeriodInDays: 30,
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

	const hasAuditorPermissions = $derived(profile.current.groups.includes(Group.AUDITOR));

	// Populate form from URL parameters (from audit logs page) or initialData
	onMount(async () => {
		if (initialData && (mode === 'view' || mode === 'edit')) {
			// Populate from initialData for view/edit modes
			form.name = initialData.name || '';
			form.enabled = initialData.enabled !== undefined ? initialData.enabled : true;
			form.bucket = initialData.bucket || '';
			form.keyPrefix = initialData.keyPrefix || '';
			form.retentionPeriodInDays = initialData.retentionPeriodInDays || 30;
			form.sourceTypes = normalizeSourceTypes(initialData.filters?.sourceTypes);

			// Populate schedule if it exists
			if (initialData.schedule) {
				form.schedule = {
					interval: initialData.schedule.interval || 'daily',
					hour: initialData.schedule.hour || 3,
					minute: initialData.schedule.minute || 0,
					day: initialData.schedule.day || 0,
					weekday: initialData.schedule.weekday || 1,
					timezone:
						initialData.schedule.timezone || Intl.DateTimeFormat().resolvedOptions().timeZone
				};
			}

			// Populate filters if they exist
			if (logType === 'llm' && initialData.llmFilters) {
				const filters = initialData.llmFilters;
				form.filters = {
					api_key_id: filters.apiKeyIDs?.join(',') ?? '',
					user_id: filters.userIDs ? filters.userIDs.join(',') : '',
					model_provider: filters.modelProviders ? filters.modelProviders.join(',') : '',
					target_model: filters.targetModels ? filters.targetModels.join(',') : '',
					request_path: filters.requestPaths ? filters.requestPaths.join(',') : '',
					response_status: filters.responseStatuses ? filters.responseStatuses.join(',') : '',
					outcome: filters.outcomes ? filters.outcomes.join(',') : '',
					user_agent: filters.userAgents ? filters.userAgents.join(',') : '',
					client_session_id: filters.clientSessionIDs ? filters.clientSessionIDs.join(',') : '',
					message_policy_triggered: filters.messagePolicyTriggered?.map(String).join(',') ?? '',
					query: filters.query ?? ''
				};
				showAdvancedOptions = true;
				return;
			}

			if (initialData.filters) {
				const filters = initialData.filters;
				form.filters = {
					api_key_id: filters.apiKeyIDs?.join(',') ?? '',
					actor: filters.actors?.join(',') ?? '',
					operation: filters.operations?.join(',') ?? '',
					mcp_server: serializeMultiValue(filters.mcpServers ?? []),
					tool: filters.tools?.join(',') ?? '',
					outcome: filters.outcomes?.join(',') ?? '',
					client: filters.clients?.join(',') ?? '',
					user_id: filters.userIDs ? filters.userIDs.join(',') : '',
					mcp_id: filters.mcpIDs ? filters.mcpIDs.join(',') : '',
					mcp_server_display_name: filters.mcpServerDisplayNames
						? filters.mcpServerDisplayNames.join(',')
						: '',
					mcp_server_catalog_entry_name: filters.mcpServerCatalogEntryNames
						? filters.mcpServerCatalogEntryNames.join(',')
						: '',
					call_type: filters.callTypes ? filters.callTypes.join(',') : '',
					call_identifier: filters.callIdentifiers ? filters.callIdentifiers.join(',') : '',
					response_status: filters.responseStatuses ? filters.responseStatuses.join(',') : '',
					session_id: filters.sessionIDs ? filters.sessionIDs.join(',') : '',
					client_name: filters.clientNames ? filters.clientNames.join(',') : '',
					client_version: filters.clientVersions ? filters.clientVersions.join(',') : '',
					client_ip: filters.clientIPs ? filters.clientIPs.join(',') : '',
					agent_provider: filters.agentProviders?.join(',') ?? '',
					status: filters.statuses?.join(',') ?? '',
					tool_name: filters.toolNames?.join(',') ?? '',
					tool_kind: filters.toolKinds?.join(',') ?? '',
					device_id: filters.deviceIDs?.join(',') ?? '',
					query: filters.query ?? ''
				};
				showAdvancedOptions = true;
			}
		} else if (mode === 'create') {
			// Populate from URL parameters for create mode
			const params = page.url.searchParams;
			if (logType === 'mcp') {
				// The audit-logs page omits event_type when no Source filter is applied; in that case
				// the all-sources default stands.
				const sourceTypes = sourceTypesFromEventTypeParam(params.get('event_type'));
				if (sourceTypes) {
					form.sourceTypes = sourceTypes;
				}
			}

			const mappedField =
				logType === 'llm'
					? ({
							api_key_id: 'api_key_id',
							user_id: 'user_id',
							user_agent: 'user_agent',
							client_session_id: 'client_session_id',
							message_policy_triggered: 'message_policy_triggered',
							model_provider: 'model_provider',
							outcome: 'outcome',
							request_path: 'request_path',
							response_status: 'response_status',
							target_model: 'target_model',
							query: 'query'
						} satisfies Record<string, keyof LLMAuditLogURLFilters>)
					: ({
							api_key_id: 'api_key_id',
							actor: 'actor',
							operation: 'operation',
							mcp_server: 'mcp_server',
							tool: 'tool',
							outcome: 'outcome',
							client: 'client',
							user_id: 'user_id',
							mcp_id: 'mcp_id',
							mcp_server_display_name: 'mcp_server_display_name',
							mcp_server_catalog_entry_name: 'mcp_server_catalog_entry_name',
							call_type: 'call_type',
							call_identifier: 'call_identifier',
							response_status: 'response_status',
							session_id: 'session_id',
							client_name: 'client_name',
							client_version: 'client_version',
							client_ip: 'client_ip',
							agent_provider: 'agent_provider',
							status: 'status',
							tool_name: 'tool_name',
							tool_kind: 'tool_kind',
							device_id: 'device_id',
							query: 'query'
						} satisfies Record<string, keyof AuditLogURLFilters>);

			let hasFilters = false;
			for (const [key, value] of Object.entries(mappedField)) {
				const param = params.get(key);
				if (param) {
					(form.filters as Record<string, string>)[value] = param;
					hasFilters = true;
				}
			}

			// Show advanced options if there are filters from the URL
			if (hasFilters) {
				showAdvancedOptions = true;
			}
		}
	});

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
					commonScheduleFilterKeys,
					mcpScheduleFilterKeys,
					localScheduleFilterKeys
				)
			)
				return;
			UserService.listAuditLogFilterOptions(id, { event_type }).then((res) => {
				filtersOptions[id] = res.options ?? [];
			});
		});
	});

	type AuditScheduleAdvancedFilterRow = {
		fieldId: string;
		filterKey:
			| 'api_key_id'
			| 'actor'
			| 'operation'
			| 'mcp_server'
			| 'tool'
			| 'client'
			| 'user_id'
			| 'mcp_id'
			| 'mcp_server_display_name'
			| 'call_type'
			| 'client_name'
			| 'response_status'
			| 'session_id'
			| 'client_ip'
			| 'mcp_server_catalog_entry_name'
			| 'user_agent'
			| 'client_session_id'
			| 'message_policy_triggered'
			| 'model_provider'
			| 'outcome'
			| 'request_path'
			| 'target_model'
			| 'agent_provider'
			| 'status'
			| 'tool_name'
			| 'tool_kind'
			| 'device_id';
		label: string;
		description: string;
		options: { id: string; label: string }[];
	};

	let auditScheduleAdvancedFilterRows = $derived.by((): AuditScheduleAdvancedFilterRow[] => {
		const resolveUserDisplayName = (id: string) =>
			usersMap.has(id) ? getUserDisplayName(usersMap, id) : id;
		const sameLabel = (d: AuditLogFilterOption) =>
			toAuditLogFilterSelectOption(d, resolveUserDisplayName);
		if (logType === 'llm') {
			return [
				{
					fieldId: 'api_key_id',
					filterKey: 'api_key_id',
					label: m.audit_usage_exports_filter_title_api_keys(),
					description: m.audit_usage_exports_filter_desc_api_keys_used(),
					options: filtersOptions['api_key_id']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'user_id',
					filterKey: 'user_id',
					label: m.audit_usage_exports_filter_title_users(),
					description: m.audit_usage_exports_filter_desc_csv_user_ids(),
					options: toStringFilterSelectOptions(filtersOptions['user_id'], resolveUserDisplayName)
				},
				{
					fieldId: 'model_provider',
					filterKey: 'model_provider',
					label: m.audit_usage_exports_filter_title_model_providers(),
					description: m.audit_usage_exports_filter_desc_csv_model_providers(),
					options: filtersOptions['model_provider']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'target_model',
					filterKey: 'target_model',
					label: m.audit_usage_exports_filter_title_target_models(),
					description: m.audit_usage_exports_filter_desc_csv_target_models(),
					options: filtersOptions['target_model']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'request_path',
					filterKey: 'request_path',
					label: m.audit_usage_exports_filter_title_request_paths(),
					description: m.audit_usage_exports_filter_desc_csv_request_paths(),
					options: filtersOptions['request_path']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'response_status',
					filterKey: 'response_status',
					label: m.audit_usage_exports_filter_title_response_status(),
					description: m.audit_usage_exports_filter_desc_csv_http_status_codes(),
					options: filtersOptions['response_status']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'outcome',
					filterKey: 'outcome',
					label: m.audit_usage_exports_filter_title_outcomes(),
					description: m.audit_usage_exports_filter_desc_csv_outcomes(),
					options: filtersOptions['outcome']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'user_agent',
					filterKey: 'user_agent',
					label: m.audit_usage_exports_filter_title_user_agents(),
					description: m.audit_usage_exports_filter_desc_csv_user_agents(),
					options: filtersOptions['user_agent']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'client_session_id',
					filterKey: 'client_session_id',
					label: m.audit_usage_exports_filter_title_client_session_ids(),
					description: m.audit_usage_exports_filter_desc_csv_client_session_ids(),
					options: filtersOptions['client_session_id']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'message_policy_triggered',
					filterKey: 'message_policy_triggered',
					label: m.audit_usage_exports_filter_title_message_policy_action(),
					description: m.audit_usage_exports_filter_desc_message_policy_triggered(),
					options: toStringFilterSelectOptions(
						filtersOptions['message_policy_triggered'],
						(value) =>
							value === 'true'
								? m.audit_usage_exports_triggered()
								: m.audit_usage_exports_not_triggered()
					)
				}
			];
		}

		return [
			// Common cross-source filters. Shown only when more than one log source is selected.
			{
				fieldId: 'actor',
				filterKey: 'actor',
				label: m.audit_usage_exports_filter_title_actors(),
				description: m.audit_usage_exports_filter_desc_users_and_devices(),
				options: toStringFilterSelectOptions(filtersOptions['actor'], resolveUserDisplayName)
			},
			{
				fieldId: 'tool',
				filterKey: 'tool',
				label: m.audit_usage_exports_filter_title_tools(),
				description: m.audit_usage_exports_filter_desc_tools_called(),
				options: filtersOptions['tool']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'mcp_server',
				filterKey: 'mcp_server',
				label: m.audit_usage_exports_filter_title_mcp_servers(),
				description: m.audit_usage_exports_filter_desc_mcp_servers_parent(),
				options: filtersOptions['mcp_server']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'operation',
				filterKey: 'operation',
				label: m.audit_usage_exports_filter_title_operations(),
				description: m.audit_usage_exports_filter_desc_mcp_operations(),
				options: filtersOptions['operation']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'outcome',
				filterKey: 'outcome',
				label: m.audit_usage_exports_filter_title_outcomes(),
				description: m.audit_usage_exports_filter_desc_outcome_values(),
				options: filtersOptions['outcome']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'client',
				filterKey: 'client',
				label: m.audit_usage_exports_filter_title_clients(),
				description: m.audit_usage_exports_filter_desc_mcp_clients_and_providers(),
				options: filtersOptions['client']?.map?.(sameLabel) ?? []
			},
			// API-key attribution is shared by every audit-log source.
			{
				fieldId: 'api_key_id',
				filterKey: 'api_key_id',
				label: m.audit_usage_exports_filter_title_api_keys(),
				description: m.audit_usage_exports_filter_desc_api_keys_used(),
				options: filtersOptions['api_key_id']?.map?.(sameLabel) ?? []
			},
			// Single-source filters. Shown only when exactly one log source is selected.
			{
				fieldId: 'agent_provider',
				filterKey: 'agent_provider',
				label: m.audit_usage_exports_filter_title_agent_providers(),
				description: m.audit_usage_exports_filter_desc_local_agent_providers(),
				options: filtersOptions['agent_provider']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'status',
				filterKey: 'status',
				label: m.audit_usage_exports_filter_title_reported_statuses(),
				description: m.audit_usage_exports_filter_desc_local_agent_statuses(),
				options: filtersOptions['status']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'tool_name',
				filterKey: 'tool_name',
				label: m.audit_usage_exports_filter_title_tool_names(),
				description: m.audit_usage_exports_filter_desc_local_tool_names(),
				options: filtersOptions['tool_name']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'tool_kind',
				filterKey: 'tool_kind',
				label: m.audit_usage_exports_filter_title_tool_kinds(),
				description: m.audit_usage_exports_filter_desc_local_tool_kinds(),
				options: filtersOptions['tool_kind']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'device_id',
				filterKey: 'device_id',
				label: m.audit_usage_exports_filter_title_device_ids(),
				description: m.audit_usage_exports_filter_desc_enrolled_device_ids(),
				options: filtersOptions['device_id']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'user_id',
				filterKey: 'user_id',
				label: m.audit_usage_exports_filter_title_user_ids(),
				description: m.audit_usage_exports_filter_desc_csv_user_ids(),
				options: toStringFilterSelectOptions(filtersOptions['user_id'], resolveUserDisplayName)
			},
			{
				fieldId: 'mcp_id',
				filterKey: 'mcp_id',
				label: m.audit_usage_exports_filter_title_server_ids(),
				description: m.audit_usage_exports_filter_desc_csv_server_ids(),
				options: filtersOptions['mcp_id']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'mcp_server_display_name',
				filterKey: 'mcp_server_display_name',
				label: m.audit_usage_exports_filter_title_server_names(),
				description: m.audit_usage_exports_filter_desc_csv_server_display_names(),
				options: filtersOptions['mcp_server_display_name']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'call_type',
				filterKey: 'call_type',
				label: m.audit_usage_exports_filter_title_call_types(),
				description: m.audit_usage_exports_filter_desc_csv_call_types(),
				options: filtersOptions['call_type']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'client_name',
				filterKey: 'client_name',
				label: m.audit_usage_exports_filter_title_client_names(),
				description: m.audit_usage_exports_filter_desc_csv_client_names(),
				options: filtersOptions['client_name']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'response_status',
				filterKey: 'response_status',
				label: m.audit_usage_exports_filter_title_response_status(),
				description: m.audit_usage_exports_filter_desc_csv_http_status_codes(),
				options: filtersOptions['response_status']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'session_id',
				filterKey: 'session_id',
				label: m.audit_usage_exports_filter_title_session_ids(),
				description: m.audit_usage_exports_filter_desc_csv_session_ids(),
				options: filtersOptions['session_id']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'client_ip',
				filterKey: 'client_ip',
				label: m.audit_usage_exports_filter_title_client_ips(),
				description: m.audit_usage_exports_filter_desc_csv_ip_addresses(),
				options: filtersOptions['client_ip']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'mcp_server_catalog_entry_name',
				filterKey: 'mcp_server_catalog_entry_name',
				label: m.audit_usage_exports_filter_title_catalog_entry_names(),
				description: m.audit_usage_exports_filter_desc_csv_catalog_entry_names(),
				options: filtersOptions['mcp_server_catalog_entry_name']?.map?.(sameLabel) ?? []
			}
		];
	});

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

			const split = (value: string | null | undefined): string[] =>
				value
					? value
							.split(',')
							.map((s) => s.trim())
							.filter((s) => s.length > 0)
					: [];
			const splitNumbers = (value: string | null | undefined): number[] =>
				split(value)
					.map((s) => Number(s))
					.filter((n) => !Number.isNaN(n));
			const splitBooleans = (value: string | null | undefined): boolean[] =>
				split(value).flatMap((item) =>
					item === 'true' ? [true] : item === 'false' ? [false] : []
				);

			if (logType === 'llm') {
				const request = {
					name: form.name,
					type: 'llm' as const,
					bucket: form.bucket,
					keyPrefix: form.keyPrefix,
					enabled: form.enabled,
					schedule: form.schedule,
					retentionPeriodInDays: form.retentionPeriodInDays,
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

				let result: ScheduledAuditLogExport | undefined = undefined;
				if (mode === 'edit' && initialData?.id) {
					result = (await AdminService.updateScheduledAuditLogExport(initialData.id, request, {
						dontLogErrors: true
					})) as ScheduledAuditLogExport;
				} else {
					result = (await AdminService.createScheduledAuditLogExport(request, {
						dontLogErrors: true
					})) as ScheduledAuditLogExport;
				}
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
				enabled: form.enabled,
				schedule: form.schedule,
				retentionPeriodInDays: form.retentionPeriodInDays,
				filters: {
					apiKeyIDs: splitNumbers(form.filters.api_key_id),
					sourceTypes: normalizeSourceTypes(form.sourceTypes),
					actors: split(form.filters.actor),
					operations: split(form.filters.operation),
					mcpServers: parseMultiValue(form.filters.mcp_server),
					tools: split(form.filters.tool),
					outcomes: split(form.filters.outcome),
					clients: split(form.filters.client),
					userIDs: form.filters.user_id ? form.filters.user_id.split(',').map((s) => s.trim()) : [],
					mcpIDs: form.filters.mcp_id ? form.filters.mcp_id.split(',').map((s) => s.trim()) : [],
					mcpServerDisplayNames: form.filters.mcp_server_display_name
						? form.filters.mcp_server_display_name.split(',').map((s) => s.trim())
						: [],
					mcpServerCatalogEntryNames: form.filters.mcp_server_catalog_entry_name
						? form.filters.mcp_server_catalog_entry_name.split(',').map((s) => s.trim())
						: [],
					callTypes: form.filters.call_type
						? form.filters.call_type.split(',').map((s) => s.trim())
						: [],
					callIdentifiers: form.filters.call_identifier
						? form.filters.call_identifier.split(',').map((s) => s.trim())
						: [],
					responseStatuses: form.filters.response_status
						? form.filters.response_status.split(',').map((s) => s.trim())
						: [],
					sessionIDs: form.filters.session_id
						? form.filters.session_id.split(',').map((s) => s.trim())
						: [],
					clientNames: form.filters.client_name
						? form.filters.client_name.split(',').map((s) => s.trim())
						: [],
					clientVersions: form.filters.client_version
						? form.filters.client_version.split(',').map((s) => s.trim())
						: [],
					clientIPs: form.filters.client_ip
						? form.filters.client_ip.split(',').map((s) => s.trim())
						: [],
					agentProviders: split(form.filters.agent_provider),
					statuses: split(form.filters.status),
					toolNames: split(form.filters.tool_name),
					toolKinds: split(form.filters.tool_kind),
					deviceIDs: split(form.filters.device_id),
					query: form.filters.query ?? ''
				}
			};

			let result: ScheduledAuditLogExport | undefined = undefined;

			if (mode === 'edit' && initialData?.id) {
				// Update existing scheduled export
				result = (await AdminService.updateScheduledAuditLogExport(initialData.id, request, {
					dontLogErrors: true
				})) as ScheduledAuditLogExport;
			} else {
				// Create new scheduled export
				result = (await AdminService.createScheduledAuditLogExport(request, {
					dontLogErrors: true
				})) as ScheduledAuditLogExport;
			}
			onSubmit(result);
		} catch (err) {
			error =
				err instanceof Error
					? err.message
					: mode === 'edit'
						? m.audit_usage_export_schedules_update_failed()
						: m.audit_usage_export_schedules_create_failed();
		} finally {
			creating = false;
		}
	}

	const hourOptions = [
		{ id: '0', label: m.audit_usage_export_schedules_hour_midnight() },
		{ id: '3', label: m.audit_usage_export_schedules_hour_3am() },
		{ id: '6', label: m.audit_usage_export_schedules_hour_6am() },
		{ id: '9', label: m.audit_usage_export_schedules_hour_9am() },
		{ id: '12', label: m.audit_usage_export_schedules_hour_noon() },
		{ id: '15', label: m.audit_usage_export_schedules_hour_3pm() },
		{ id: '18', label: m.audit_usage_export_schedules_hour_6pm() },
		{ id: '21', label: m.audit_usage_export_schedules_hour_9pm() }
	];

	const selectClasses = 'text-input-filled bg-base-200 dark:bg-base-100';
	const selectRootClass = 'w-full md:max-w-xs';
	const commonScheduleFilterKeys = new Set<string>(COMMON_FILTER_KEYS);
	const mcpScheduleFilterKeys = new Set([
		'mcp_id',
		'mcp_server_display_name',
		'mcp_server_catalog_entry_name',
		'call_type',
		'call_identifier',
		'client_name',
		'client_version',
		'response_status'
	]);
	const localScheduleFilterKeys = new Set([
		'agent_provider',
		'status',
		'tool_name',
		'tool_kind',
		'device_id'
	]);
	const visibleScheduleFilterRows = $derived(
		filterVisibleExportFields(
			form,
			auditScheduleAdvancedFilterRows,
			commonScheduleFilterKeys,
			mcpScheduleFilterKeys,
			localScheduleFilterKeys
		)
	);

	function toggleSourceType(sourceType: string, checked: boolean) {
		form.sourceTypes = normalizeSourceTypes(
			checked
				? [...form.sourceTypes, sourceType]
				: form.sourceTypes.filter((value) => value !== sourceType)
		);
		clearSourceScopedFilters(form.filters);
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
		<div class="space-y-4">
			<h3 class="text-lg font-semibold">
				{#if mode === 'view'}
					{m.audit_usage_export_schedules_details()}
				{:else if mode === 'edit'}
					{m.audit_usage_audit_logs_edit_scheduled_export()}
				{:else}
					{m.audit_usage_exports_basic_information()}
				{/if}
			</h3>

			<div class="grid grid-cols-1 gap-6 md:grid-cols-2">
				<div class="flex flex-col gap-1">
					<label class="text-sm font-medium" for="name"
						>{m.audit_usage_export_schedules_name_label()}</label
					>
					<input
						class="text-input-filled"
						id="name"
						bind:value={form.name}
						placeholder="daily-audit-export"
						required={mode !== 'view'}
						readonly={mode === 'view'}
					/>
					<p class="text-muted-content text-xs">{m.audit_usage_export_schedules_name_help()}</p>
				</div>
				<div class="flex flex-col gap-1">
					<label class="text-sm font-medium" for="bucket"
						>{m.audit_usage_exports_bucket_label()}</label
					>
					<input
						class="text-input-filled"
						id="bucket"
						bind:value={form.bucket}
						placeholder="my-audit-exports"
						required={mode !== 'view'}
						readonly={mode === 'view'}
					/>
					<p class="text-muted-content text-xs">{m.audit_usage_exports_bucket_help()}</p>
				</div>
			</div>

			<div class="flex flex-col gap-1">
				<label class="text-sm font-medium" for="keyPrefix"
					>{m.audit_usage_exports_key_prefix_label()}</label
				>
				<input
					class="text-input-filled"
					id="keyPrefix"
					bind:value={form.keyPrefix}
					placeholder={m.audit_usage_exports_key_prefix_placeholder({ prefix: defaultKeyPrefix })}
					readonly={mode === 'view'}
				/>
				<p class="text-muted-content text-xs">
					{m.audit_usage_exports_key_prefix_help({ prefix: defaultKeyPrefix })}
				</p>
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
									onchange={(event) => toggleSourceType(sourceType, event.currentTarget.checked)}
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

		<!-- Schedule Configuration -->
		<div class="flex flex-col gap-4">
			<h3 class="text-lg font-semibold">{m.audit_usage_export_schedules_configuration()}</h3>

			<div class="flex flex-wrap gap-4">
				<Select
					id="schedule-interval"
					class={selectClasses}
					classes={{ root: selectRootClass }}
					options={[
						{ id: 'hourly', label: m.audit_usage_export_schedules_hourly() },
						{ id: 'daily', label: m.audit_usage_export_schedules_daily() },
						{ id: 'weekly', label: m.audit_usage_export_schedules_weekly() },
						{ id: 'monthly', label: m.audit_usage_export_schedules_monthly() }
					]}
					selected={form.schedule.interval}
					onSelect={(value) => {
						if (mode !== 'view') {
							form.schedule.interval = value.id;
						}
					}}
					disabled={mode === 'view'}
				/>

				{#if form.schedule.interval === 'hourly'}
					<Select
						id="schedule-minute"
						class={selectClasses}
						classes={{ root: selectRootClass }}
						options={[
							{ id: '0', label: m.audit_usage_export_schedules_on_the_hour() },
							{ id: '15', label: m.audit_usage_export_schedules_minutes_past({ minutes: 15 }) },
							{ id: '30', label: m.audit_usage_export_schedules_minutes_past({ minutes: 30 }) },
							{ id: '45', label: m.audit_usage_export_schedules_minutes_past({ minutes: 45 }) }
						]}
						selected={form.schedule.minute.toString()}
						onSelect={(value) => {
							form.schedule.minute = parseInt(value.id);
						}}
					/>
				{/if}

				{#if form.schedule.interval === 'daily'}
					<Select
						id="schedule-hour"
						class={selectClasses}
						classes={{ root: selectRootClass }}
						options={hourOptions}
						selected={form.schedule.hour.toString()}
						onSelect={(value) => {
							form.schedule.hour = parseInt(value.id);
						}}
					/>
					{#if form.schedule.timezone && form.schedule.timezone !== defaultTimezone}
						<div class="flex items-center gap-1">
							<GlobeIcon class="text-muted-foreground mr-1 h-4 w-4" />
							<span class="text-muted-foreground text-sm">{form.schedule.timezone}</span>
						</div>
					{/if}
				{/if}

				{#if form.schedule.interval === 'weekly'}
					<Select
						id="schedule-weekday"
						class={selectClasses}
						classes={{ root: selectRootClass }}
						options={[
							{ id: '0', label: m.audit_usage_export_schedules_sunday() },
							{ id: '1', label: m.audit_usage_export_schedules_monday() },
							{ id: '2', label: m.audit_usage_export_schedules_tuesday() },
							{ id: '3', label: m.audit_usage_export_schedules_wednesday() },
							{ id: '4', label: m.audit_usage_export_schedules_thursday() },
							{ id: '5', label: m.audit_usage_export_schedules_friday() },
							{ id: '6', label: m.audit_usage_export_schedules_saturday() }
						]}
						selected={form.schedule.weekday.toString()}
						onSelect={(value) => {
							form.schedule.weekday = parseInt(value.id);
						}}
					/>
					<Select
						id="schedule-hour"
						class={selectClasses}
						classes={{ root: selectRootClass }}
						options={hourOptions}
						selected={form.schedule.hour.toString()}
						onSelect={(value) => {
							form.schedule.hour = parseInt(value.id);
						}}
					/>
					{#if form.schedule.timezone && form.schedule.timezone !== defaultTimezone}
						<div class="flex items-center gap-1">
							<GlobeIcon class="text-muted-foreground mr-1 h-4 w-4" />
							<span class="text-muted-foreground text-sm">{form.schedule.timezone}</span>
						</div>
					{/if}
				{/if}

				{#if form.schedule.interval === 'monthly'}
					<Select
						id="schedule-day"
						class={selectClasses}
						classes={{ root: selectRootClass }}
						options={[
							{ id: '0', label: m.audit_usage_export_schedules_day_1st() },
							{ id: '1', label: m.audit_usage_export_schedules_day_2nd() },
							{ id: '2', label: m.audit_usage_export_schedules_day_3rd() },
							{ id: '4', label: m.audit_usage_export_schedules_day_5th() },
							{ id: '14', label: m.audit_usage_export_schedules_day_15th() },
							{ id: '19', label: m.audit_usage_export_schedules_day_20th() },
							{ id: '24', label: m.audit_usage_export_schedules_day_25th() },
							{ id: '-1', label: m.audit_usage_export_schedules_day_last() }
						]}
						selected={form.schedule.day.toString()}
						onSelect={(value) => {
							form.schedule.day = parseInt(value.id);
						}}
					/>
					<Select
						id="schedule-hour"
						class={selectClasses}
						classes={{ root: selectRootClass }}
						options={hourOptions}
						selected={form.schedule.hour.toString()}
						onSelect={(value) => {
							form.schedule.hour = parseInt(value.id);
						}}
					/>
					{#if form.schedule.timezone && form.schedule.timezone !== defaultTimezone}
						<div class="flex items-center gap-1">
							<GlobeIcon class="text-muted-foreground mr-1 h-4 w-4" />
							<span class="text-muted-foreground text-sm">{form.schedule.timezone}</span>
						</div>
					{/if}
				{/if}
			</div>
		</div>

		<div class="space-y-4">
			<h3 class="text-lg font-semibold">{m.audit_usage_exports_time_range()}</h3>
			<p class="text-sm text-gray-600">
				{m.audit_usage_export_schedules_time_range_help()}
			</p>
			<div class="flex flex-col gap-1">
				<Select
					id="schedule-retention-period"
					class={twMerge(selectClasses, 'w-full max-w-xs')}
					options={[
						{ id: '1', label: m.audit_usage_export_schedules_last_1_day() },
						{ id: '3', label: m.audit_usage_export_schedules_last_n_days({ days: 3 }) },
						{ id: '7', label: m.audit_usage_export_schedules_last_n_days({ days: 7 }) },
						{ id: '30', label: m.audit_usage_export_schedules_last_n_days({ days: 30 }) },
						{ id: '60', label: m.audit_usage_export_schedules_last_n_days({ days: 60 }) },
						{ id: '90', label: m.audit_usage_export_schedules_last_n_days({ days: 90 }) },
						{ id: '-1', label: m.audit_usage_export_schedules_all_logs() }
					]}
					selected={form.retentionPeriodInDays.toString()}
					onSelect={(value) => {
						form.retentionPeriodInDays = parseInt(value.id);
					}}
				/>
			</div>
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
						{m.audit_usage_export_schedules_leave_filters_empty()}
					</p>

					<div class="flex flex-col gap-1">
						<label class="text-sm font-medium" for="query"
							>{m.audit_usage_exports_search_query()}</label
						>
						<input
							id="query"
							class={selectClasses}
							bind:value={form.filters.query}
							placeholder={m.audit_usage_exports_search_placeholder()}
							readonly={isViewMode}
							disabled={isViewMode}
						/>
						<p class="text-muted-content text-xs">
							{m.audit_usage_export_schedules_search_help()}
						</p>
					</div>

					{#snippet auditScheduleAdvancedFilterField(row: AuditScheduleAdvancedFilterRow)}
						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for={row.fieldId}>{row.label}</label>
							<Select
								id={row.fieldId}
								class={selectClasses}
								classes={{
									root: 'w-full',
									clear: 'hover:bg-base-400 bg-transparent'
								}}
								options={row.options}
								bind:selected={
									() => form.filters[row.filterKey] ?? '',
									(v) => {
										form.filters[row.filterKey] = v ?? '';
									}
								}
								disabled={isViewMode}
								multiple
								valueFormat={row.filterKey === 'mcp_server' ? 'json' : 'comma-separated'}
							/>
							<p class="text-muted-content text-xs">{row.description}</p>
						</div>
					{/snippet}

					<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
						{#each visibleScheduleFilterRows as row (row.fieldId)}
							{@render auditScheduleAdvancedFilterField(row)}
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
							: m.audit_usage_export_schedules_creating()}
					{:else}
						{mode === 'edit' ? m.core_save_changes() : m.audit_usage_export_schedules_create()}
					{/if}
				</button>
			{/if}
		</div>
	</form>
</div>
