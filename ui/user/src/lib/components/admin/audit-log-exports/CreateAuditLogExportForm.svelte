<script lang="ts">
	import { page } from '$app/state';
	import type { DateRange } from '$lib/components/Calendar.svelte';
	import Select from '$lib/components/Select.svelte';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, Group, type AuditLogURLFilters } from '$lib/services';
	import type { AuditLogExport, OrgUser } from '$lib/services/admin/types';
	import { profile } from '$lib/stores';
	import { subDays, set } from 'date-fns';
	import { TriangleAlert, ChevronDown, ChevronUp } from 'lucide-svelte';
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
		| 'session_id';

	type AuditLogExportFilterFieldConfig = {
		filterKey: AuditLogExportMultiSelectFilterKey;
		title: string;
		description: string;
		placeholder: string;
		useUserDisplayNames?: boolean;
	};

	const AUDIT_LOG_EXPORT_FILTER_FIELDS: AuditLogExportFilterFieldConfig[] = [
		{
			filterKey: 'user_id',
			title: 'Users',
			description: 'List of users',
			placeholder: 'user1,user2',
			useUserDisplayNames: true
		},
		{
			filterKey: 'mcp_id',
			title: 'Server IDs',
			description: 'List of server IDs',
			placeholder: 'server1,server2'
		},
		{
			filterKey: 'mcp_server_display_name',
			title: 'Server Names',
			description: 'List of server display names',
			placeholder: 'server-name-1,server-name-2'
		},
		{
			filterKey: 'call_type',
			title: 'Call Types',
			description: 'List of call types',
			placeholder: 'tools/call,resources/read'
		},
		{
			filterKey: 'client_name',
			title: 'Client Names',
			description: 'List of client names',
			placeholder: 'client1,client2'
		},
		{
			filterKey: 'response_status',
			title: 'Response Status',
			description: 'List of HTTP status codes',
			placeholder: '200,400,500'
		},
		{
			filterKey: 'session_id',
			title: 'Session IDs',
			description: 'List of session IDs',
			placeholder: 'session1,session2'
		},
		{
			filterKey: 'client_ip',
			title: 'Client IPs',
			description: 'List of IP addresses',
			placeholder: '192.168.1.1,10.0.0.1'
		},
		{
			filterKey: 'call_identifier',
			title: 'Call Identifier',
			description: 'List of call identifiers',
			placeholder: 'call-identifier-1,call-identifier-2'
		},
		{
			filterKey: 'client_version',
			title: 'Client Versions',
			description: 'List of client versions',
			placeholder: 'client-version-1,client-version-2'
		},
		{
			filterKey: 'mcp_server_catalog_entry_name',
			title: 'Catalog Entry Names',
			description: 'List of catalog entry names',
			placeholder: 'workspace-id-1,workspace-id-2'
		}
	];

	interface Props {
		onCancel: () => void;
		onSubmit: (result?: AuditLogExport) => void;
		mode?: 'create' | 'view' | 'edit';
		initialData?: AuditLogExport;
	}

	let { onCancel, onSubmit, mode = 'create', initialData }: Props = $props();

	let showAdvancedOptions = $state(false);
	let isViewMode = $derived(mode === 'view');

	const hasAuditorPermissions = $derived(profile.current.groups.includes(Group.AUDITOR));

	// Form state
	let form = $state({
		name: '',
		bucket: '',
		keyPrefix: '',
		startTime: subDays(new Date(), 7),
		endTime: set(new Date(), { milliseconds: 0, seconds: 59 }),
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
			query: ''
		} as Partial<AuditLogURLFilters>
	});

	let creating = $state(false);
	let error = $state('');

	let filtersIds = [
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
		'response_status'
	];

	let usersMap = new SvelteMap<string, OrgUser>();
	let filtersOptions: Record<string, string[]> = $state({});

	onMount(async () => {
		if (initialData && (mode === 'view' || mode === 'edit')) {
			form.name = initialData.name || '';
			form.bucket = initialData.bucket || '';
			form.keyPrefix = initialData.keyPrefix || '';
			form.startTime = initialData.startTime ? new Date(initialData.startTime) : form.startTime;
			form.endTime = initialData.endTime ? new Date(initialData.endTime) : form.endTime;

			if (initialData.filters) {
				form.filters = {
					user_id: join(initialData.filters.userIDs),
					mcp_id: join(initialData.filters.mcpIDs),
					mcp_server_display_name: join(initialData.filters.mcpServerDisplayNames),
					mcp_server_catalog_entry_name: join(initialData.filters.mcpServerCatalogEntryNames),
					call_type: join(initialData.filters.callTypes),
					call_identifier: join(initialData.filters.callIdentifiers),
					response_status: join(initialData.filters.responseStatuses),
					session_id: join(initialData.filters.sessionIDs),
					client_name: join(initialData.filters.clientNames),
					client_version: join(initialData.filters.clientVersions),
					client_ip: join(initialData.filters.clientIPs)
				};
				showAdvancedOptions = true;
			}
		} else if (mode === 'create') {
			// Populate from URL parameters for create mode
			const params = page.url.searchParams;

			// Set time range if provided
			const startTime = params.get('startTime');
			const endTime = params.get('endTime');
			if (startTime) {
				form.startTime = new Date(startTime);
			}
			if (endTime) {
				form.endTime = new Date(endTime);
			}

			// Set filters if provided
			const filterKeys = [
				'user_id',
				'mcp_id',
				'mcp_server_display_name',
				'mcp_server_catalog_entry_name',
				'call_type',
				'call_identifier',
				'client_name',
				'client_version',
				'client_ip',
				'response_status',
				'session_id'
			];

			let hasFilters = false;
			filterKeys.forEach((key) => {
				const value = params.get(key);
				if (value && key in form.filters) {
					(form.filters as Record<string, string>)[key] = value;
					hasFilters = true;
				}
			});

			// Show advanced options if there are filters from the URL
			if (hasFilters) {
				showAdvancedOptions = true;
			}
		}
	});

	$effect(() => {
		AdminService.listUsers().then((res) => {
			res.forEach((user) => {
				usersMap.set(user.id, user);
			});
		});
	});

	$effect(() => {
		filtersIds.forEach((id) => {
			AdminService.listAuditLogFilterOptions(id).then((res) => {
				filtersOptions[id] = res.options ?? [];
			});
		});
	});

	function join(array: string[] | undefined): string {
		return array ? array.join(',') : '';
	}

	function split(value: string | null | undefined): string[] {
		return value ? value.split(',').map((s) => s.trim()) : [];
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

			// Prepare the request
			const request = {
				name: form.name,
				bucket: form.bucket,
				keyPrefix: form.keyPrefix,
				startTime: form.startTime.toISOString(),
				endTime: form.endTime.toISOString(),
				filters: {
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
					clientIPs: split(form.filters.client_ip)
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
		return opts.map((d) => ({ id: d, label: d }));
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
					placeholder="Leave empty for default: mcp-audit-logs/YYYY/MM/DD/"
					readonly={isViewMode}
					disabled={isViewMode}
				/>
				{#if (isViewMode && form.keyPrefix) || !isViewMode}
					<p class="text-muted-content text-xs">
						Path prefix within the bucket. If empty, defaults to "mcp-audit-logs/YYYY/MM/DD/" format
						based on current date.
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

					{#snippet auditLogExportFilterSelect(
						filterKey: AuditLogExportMultiSelectFilterKey,
						title: string,
						description: string,
						placeholder: string,
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
								{placeholder}
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
						{#each AUDIT_LOG_EXPORT_FILTER_FIELDS as field (field.filterKey)}
							{@render auditLogExportFilterSelect(
								field.filterKey,
								field.title,
								field.description,
								field.placeholder,
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
