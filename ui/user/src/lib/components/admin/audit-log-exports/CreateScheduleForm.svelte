<script lang="ts">
	import { page } from '$app/state';
	import Select from '$lib/components/Select.svelte';
	import {
		ALL_SOURCE_TYPES,
		clearSourceScopedFilters,
		COMMON_FILTER_KEYS,
		eventTypeParamFromSourceTypes,
		filterVisibleExportFields,
		normalizeSourceTypes,
		sourceTypeLabels,
		sourceTypesFromEventTypeParam
	} from '$lib/components/admin/audit-log-exports/filterFields';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		type LLMAuditLogURLFilters,
		type OrgUser,
		type ScheduledAuditLogExport,
		AdminService,
		Group,
		UserService,
		type AuditLogURLFilters
	} from '$lib/services';
	import { profile } from '$lib/stores';
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
					actor: filters.actors?.join(',') ?? '',
					operation: filters.operations?.join(',') ?? '',
					mcp_server: filters.mcpServers?.join(',') ?? '',
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
	let filtersOptions: Record<string, string[]> = $state({});

	$effect(() => {
		UserService.listUsers().then((res) => {
			res.forEach((user) => {
				usersMap.set(user.id, user);
			});
		});
	});

	$effect(() => {
		const event_type = eventTypeParamFromSourceTypes(form.sourceTypes);
		const multiSource = form.sourceTypes.length > 1;
		const singleSource = form.sourceTypes.length === 1;
		filtersIds.forEach((id) => {
			if (logType === 'llm') {
				AdminService.listLLMAuditLogFilterOptions(id).then((res) => {
					filtersOptions[id] = res.options ?? [];
				});
				return;
			}
			// Match visibleScheduleFilterRows: common filters for multi-source, source-specific for the
			// matching single source, and shared columns for any single source.
			if (commonScheduleFilterKeys.has(id)) {
				if (!multiSource) return;
			} else if (localScheduleFilterKeys.has(id)) {
				if (!(singleSource && form.sourceTypes.includes('local_agent_tool_call'))) return;
			} else if (mcpScheduleFilterKeys.has(id)) {
				if (!(singleSource && form.sourceTypes.includes('mcp'))) return;
			} else if (!singleSource) {
				// Shared columns (user_id, session_id, client_ip).
				return;
			}
			UserService.listAuditLogFilterOptions(id, { event_type }).then((res) => {
				filtersOptions[id] = res.options ?? [];
			});
		});
	});

	type AuditScheduleAdvancedFilterRow = {
		fieldId: string;
		filterKey:
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
		const sameLabel = (d: string) => ({ id: d, label: d });
		if (logType === 'llm') {
			return [
				{
					fieldId: 'user_id',
					filterKey: 'user_id',
					label: 'Users',
					description: 'Comma-separated user IDs',
					options:
						filtersOptions['user_id']?.map?.((d) => ({
							id: d,
							label: usersMap.get(d)?.displayName ?? d
						})) ?? []
				},
				{
					fieldId: 'model_provider',
					filterKey: 'model_provider',
					label: 'Model Providers',
					description: 'Comma-separated model providers',
					options: filtersOptions['model_provider']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'target_model',
					filterKey: 'target_model',
					label: 'Target Models',
					description: 'Comma-separated target models',
					options: filtersOptions['target_model']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'request_path',
					filterKey: 'request_path',
					label: 'Request Paths',
					description: 'Comma-separated request paths',
					options: filtersOptions['request_path']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'response_status',
					filterKey: 'response_status',
					label: 'Response Status',
					description: 'Comma-separated HTTP status codes',
					options: filtersOptions['response_status']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'outcome',
					filterKey: 'outcome',
					label: 'Outcomes',
					description: 'Comma-separated outcomes',
					options: filtersOptions['outcome']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'user_agent',
					filterKey: 'user_agent',
					label: 'User Agents',
					description: 'Comma-separated user agents',
					options: filtersOptions['user_agent']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'client_session_id',
					filterKey: 'client_session_id',
					label: 'Client Session IDs',
					description: 'Comma-separated client session IDs',
					options: filtersOptions['client_session_id']?.map?.(sameLabel) ?? []
				},
				{
					fieldId: 'message_policy_triggered',
					filterKey: 'message_policy_triggered',
					label: 'Message Policy Action',
					description: 'Filter by whether a message policy was triggered',
					options:
						filtersOptions['message_policy_triggered']?.map?.((value) => ({
							id: value,
							label: value === 'true' ? 'Triggered' : 'Not triggered'
						})) ?? []
				}
			];
		}

		return [
			// Common cross-source filters. Shown only when more than one log source is selected.
			{
				fieldId: 'actor',
				filterKey: 'actor',
				label: 'Actors',
				description: 'Users and enrolled devices',
				options:
					filtersOptions['actor']?.map?.((d) => ({
						id: d,
						label: usersMap.get(d)?.displayName ?? d
					})) ?? []
			},
			{
				fieldId: 'tool',
				filterKey: 'tool',
				label: 'Tools',
				description: 'Tools called (MCP call identifiers and local tool names)',
				options: filtersOptions['tool']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'mcp_server',
				filterKey: 'mcp_server',
				label: 'MCP Servers',
				description: 'MCP servers (and the parent server of local tool calls)',
				options: filtersOptions['mcp_server']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'operation',
				filterKey: 'operation',
				label: 'Operations',
				description: 'MCP operations; local tool calls are all tools/call',
				options: filtersOptions['operation']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'outcome',
				filterKey: 'outcome',
				label: 'Outcomes',
				description: 'success, failure, denied, timeout, or unknown',
				options: filtersOptions['outcome']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'client',
				filterKey: 'client',
				label: 'Clients',
				description: 'MCP clients and local-agent providers',
				options: filtersOptions['client']?.map?.(sameLabel) ?? []
			},
			// Single-source filters. Shown only when exactly one log source is selected.
			{
				fieldId: 'agent_provider',
				filterKey: 'agent_provider',
				label: 'Agent Providers',
				description: 'Local-agent providers',
				options: filtersOptions['agent_provider']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'status',
				filterKey: 'status',
				label: 'Reported Statuses',
				description: 'Local-agent statuses',
				options: filtersOptions['status']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'tool_name',
				filterKey: 'tool_name',
				label: 'Tool Names',
				description: 'Local tool names',
				options: filtersOptions['tool_name']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'tool_kind',
				filterKey: 'tool_kind',
				label: 'Tool Kinds',
				description: 'Local tool kinds',
				options: filtersOptions['tool_kind']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'device_id',
				filterKey: 'device_id',
				label: 'Device IDs',
				description: 'Enrolled device IDs',
				options: filtersOptions['device_id']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'user_id',
				filterKey: 'user_id',
				label: 'User IDs',
				description: 'Comma-separated user IDs',
				options:
					filtersOptions['user_id']?.map?.((d) => ({
						id: d,
						label: usersMap.get(d)?.displayName ?? d
					})) ?? []
			},
			{
				fieldId: 'mcp_id',
				filterKey: 'mcp_id',
				label: 'Server IDs',
				description: 'Comma-separated server IDs',
				options: filtersOptions['mcp_id']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'mcp_server_display_name',
				filterKey: 'mcp_server_display_name',
				label: 'Server Names',
				description: 'Comma-separated server display names',
				options: filtersOptions['mcp_server_display_name']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'call_type',
				filterKey: 'call_type',
				label: 'Call Types',
				description: 'Comma-separated call types',
				options: filtersOptions['call_type']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'client_name',
				filterKey: 'client_name',
				label: 'Client Names',
				description: 'Comma-separated client names',
				options: filtersOptions['client_name']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'response_status',
				filterKey: 'response_status',
				label: 'Response Status',
				description: 'Comma-separated HTTP status codes',
				options: filtersOptions['response_status']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'session_id',
				filterKey: 'session_id',
				label: 'Session IDs',
				description: 'Comma-separated session IDs',
				options: filtersOptions['session_id']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'client_ip',
				filterKey: 'client_ip',
				label: 'Client IPs',
				description: 'Comma-separated IP addresses',
				options: filtersOptions['client_ip']?.map?.(sameLabel) ?? []
			},
			{
				fieldId: 'mcp_server_catalog_entry_name',
				filterKey: 'mcp_server_catalog_entry_name',
				label: 'Catalog Entry Names',
				description: 'Comma-separated catalog entry names',
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
				throw new Error('Name is required');
			}
			if (!form.bucket) {
				throw new Error('Bucket name is required');
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
				throw new Error('At least one log source must be selected');
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
					sourceTypes: normalizeSourceTypes(form.sourceTypes),
					actors: split(form.filters.actor),
					operations: split(form.filters.operation),
					mcpServers: split(form.filters.mcp_server),
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
					: `Failed to ${mode === 'edit' ? 'update' : 'create'} export schedule`;
		} finally {
			creating = false;
		}
	}

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
					Exported logs will not include request/response headers and body information. Auditor role
					is required to access this data.
				</div>
			</div>
		{/if}

		<!-- Basic Information -->
		<div class="space-y-4">
			<h3 class="text-lg font-semibold">
				{#if mode === 'view'}
					Scheduled Export Details
				{:else if mode === 'edit'}
					Edit Scheduled Export
				{:else}
					Basic Information
				{/if}
			</h3>

			<div class="grid grid-cols-1 gap-6 md:grid-cols-2">
				<div class="flex flex-col gap-1">
					<label class="text-sm font-medium" for="name">Schedule Name</label>
					<input
						class="text-input-filled"
						id="name"
						bind:value={form.name}
						placeholder="daily-audit-export"
						required={mode !== 'view'}
						readonly={mode === 'view'}
					/>
					<p class="text-muted-content text-xs">Unique name for this export schedule</p>
				</div>
				<div class="flex flex-col gap-1">
					<label class="text-sm font-medium" for="bucket">Bucket Name</label>
					<input
						class="text-input-filled"
						id="bucket"
						bind:value={form.bucket}
						placeholder="my-audit-exports"
						required={mode !== 'view'}
						readonly={mode === 'view'}
					/>
					<p class="text-muted-content text-xs">Storage bucket name where exports will be saved</p>
				</div>
			</div>

			<div class="flex flex-col gap-1">
				<label class="text-sm font-medium" for="keyPrefix">Key Prefix (Optional)</label>
				<input
					class="text-input-filled"
					id="keyPrefix"
					bind:value={form.keyPrefix}
					placeholder={`Leave empty for default: ${defaultKeyPrefix}/YYYY/MM/DD/`}
					readonly={mode === 'view'}
				/>
				<p class="text-muted-content text-xs">
					Path prefix within the bucket. If empty, defaults to "{defaultKeyPrefix}/YYYY/MM/DD/"
					format based on current date.
				</p>
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
									onchange={(event) => toggleSourceType(sourceType, event.currentTarget.checked)}
								/>
								{sourceTypeLabels[sourceType]}
							</label>
						{/each}
					</div>
					{#if !isViewMode}
						<p class="text-muted-content text-xs">
							Which audit-log source(s) to export. Select both to include MCP and local-agent
							tool-call logs in the same export. At least one source is required.
						</p>
					{/if}
				</div>
			{/if}
		</div>

		<!-- Schedule Configuration -->
		<div class="flex flex-col gap-4">
			<h3 class="text-lg font-semibold">Schedule Configuration</h3>

			<div class="flex flex-wrap gap-4">
				<Select
					id="schedule-interval"
					class={selectClasses}
					classes={{ root: selectRootClass }}
					options={[
						{ id: 'hourly', label: 'Hourly' },
						{ id: 'daily', label: 'Daily' },
						{ id: 'weekly', label: 'Weekly' },
						{ id: 'monthly', label: 'Monthly' }
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
							{ id: '0', label: 'on the hour' },
							{ id: '15', label: '15 minutes past' },
							{ id: '30', label: '30 minutes past' },
							{ id: '45', label: '45 minutes past' }
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
						options={[
							{ id: '0', label: 'midnight' },
							{ id: '3', label: '3 AM' },
							{ id: '6', label: '6 AM' },
							{ id: '9', label: '9 AM' },
							{ id: '12', label: 'noon' },
							{ id: '15', label: '3 PM' },
							{ id: '18', label: '6 PM' },
							{ id: '21', label: '9 PM' }
						]}
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
							{ id: '0', label: 'Sunday' },
							{ id: '1', label: 'Monday' },
							{ id: '2', label: 'Tuesday' },
							{ id: '3', label: 'Wednesday' },
							{ id: '4', label: 'Thursday' },
							{ id: '5', label: 'Friday' },
							{ id: '6', label: 'Saturday' }
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
						options={[
							{ id: '0', label: 'midnight' },
							{ id: '3', label: '3 AM' },
							{ id: '6', label: '6 AM' },
							{ id: '9', label: '9 AM' },
							{ id: '12', label: 'noon' },
							{ id: '15', label: '3 PM' },
							{ id: '18', label: '6 PM' },
							{ id: '21', label: '9 PM' }
						]}
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
							{ id: '0', label: '1st' },
							{ id: '1', label: '2nd' },
							{ id: '2', label: '3rd' },
							{ id: '4', label: '5th' },
							{ id: '14', label: '15th' },
							{ id: '19', label: '20th' },
							{ id: '24', label: '25th' },
							{ id: '-1', label: 'last day' }
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
						options={[
							{ id: '0', label: 'midnight' },
							{ id: '3', label: '3 AM' },
							{ id: '6', label: '6 AM' },
							{ id: '9', label: '9 AM' },
							{ id: '12', label: 'noon' },
							{ id: '15', label: '3 PM' },
							{ id: '18', label: '6 PM' },
							{ id: '21', label: '9 PM' }
						]}
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
			<h3 class="text-lg font-semibold">Time Range</h3>
			<p class="text-sm text-gray-600">
				Define how many days of logs to include in each scheduled export. Each export will include
				logs from the last X days relative to the export time.
			</p>
			<div class="flex flex-col gap-1">
				<Select
					id="schedule-retention-period"
					class={twMerge(selectClasses, 'w-full max-w-xs')}
					options={[
						{ id: '1', label: 'Last 1 day' },
						{ id: '3', label: 'Last 3 days' },
						{ id: '7', label: 'Last 7 days' },
						{ id: '30', label: 'Last 30 days' },
						{ id: '60', label: 'Last 60 days' },
						{ id: '90', label: 'Last 90 days' },
						{ id: '-1', label: 'All logs' }
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
						Leave filters empty to export all logs in each scheduled period
					</p>

					<div class="flex flex-col gap-1">
						<label class="text-sm font-medium" for="query">Search Query</label>
						<input
							id="query"
							class={selectClasses}
							bind:value={form.filters.query}
							placeholder="Search audit logs"
							readonly={isViewMode}
							disabled={isViewMode}
						/>
						<p class="text-muted-content text-xs">
							Free-text search to apply to each exported audit-log period
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
				{mode === 'view' ? 'Back' : 'Cancel'}
			</button>
			{#if mode !== 'view'}
				<button type="submit" class="btn btn-primary" disabled={creating}>
					{#if creating}
						<Loading class="size-4" />
						{mode === 'edit' ? 'Saving Changes...' : 'Creating Schedule...'}
					{:else}
						{mode === 'edit' ? 'Save Changes' : 'Create Schedule'}
					{/if}
				</button>
			{/if}
		</div>
	</form>
</div>
