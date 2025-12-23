<script lang="ts">
	import { onMount } from 'svelte';
	import { profile } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';
	import { slide } from 'svelte/transition';
	import { subDays, set } from 'date-fns';
	import { AlertTriangle, LoaderCircle, ChevronDown, ChevronUp } from 'lucide-svelte';
	import { page } from '$app/state';
	import { AdminService, Group, type AuditLogURLFilters } from '$lib/services';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import type { DateRange } from '$lib/components/Calendar.svelte';
	import type { AuditLogExport, OrgUser } from '$lib/services/admin/types';
	import Select from '$lib/components/Select.svelte';
	import { SvelteMap } from 'svelte/reactivity';

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
</script>

<div class="dark:bg-surface2 bg-background rounded-md p-6 shadow-sm">
	<form
		class="space-y-8"
		onsubmit={(e) => {
			e.preventDefault();
			handleSubmit();
		}}
	>
		{#if !hasAuditorPermissions}
			<div
				class="flex items-start gap-3 rounded-md border border-yellow-500 bg-yellow-500/10 p-4 dark:bg-yellow-500/10"
			>
				<AlertTriangle class="size-5 text-yellow-500 dark:text-yellow-500" />
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
							isViewMode && '[color:currentColor] disabled:opacity-100'
						)}
						id="name"
						bind:value={form.name}
						placeholder="audit-export-2024"
						required={!isViewMode}
						readonly={isViewMode}
						disabled={isViewMode}
					/>
					{#if (isViewMode && form.name) || !isViewMode}
						<p class="text-on-surface1 text-xs">Unique name for this export</p>
					{/if}
				</div>
				<div class="flex flex-col gap-1">
					<label class="text-sm font-medium" for="bucket">Bucket Name</label>
					<input
						class={twMerge(
							'text-input-filled',
							isViewMode && '[color:currentColor] disabled:opacity-100'
						)}
						id="bucket"
						bind:value={form.bucket}
						placeholder="my-audit-exports"
						required={!isViewMode}
						readonly={isViewMode}
						disabled={isViewMode}
					/>
					{#if (isViewMode && form.bucket) || !isViewMode}
						<p class="text-on-surface1 text-xs">Storage bucket name where exports will be saved</p>
					{/if}
				</div>
			</div>

			<div class="flex flex-col gap-1">
				<label class="text-sm font-medium" for="keyPrefix">Key Prefix (Optional)</label>
				<input
					class={twMerge(
						'text-input-filled',
						isViewMode && '[color:currentColor] disabled:opacity-100'
					)}
					id="keyPrefix"
					bind:value={form.keyPrefix}
					placeholder="Leave empty for default: mcp-audit-logs/YYYY/MM/DD/"
					readonly={isViewMode}
					disabled={isViewMode}
				/>
				{#if (isViewMode && form.keyPrefix) || !isViewMode}
					<p class="text-on-surface1 text-xs">
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

					<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="user_id">Users</label>
							<Select
								class={twMerge(
									'dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner',
									isViewMode && '[color:currentColor] disabled:opacity-100'
								)}
								classes={{
									root: 'w-full',
									clear: 'hover:bg-surface3 bg-transparent'
								}}
								options={filtersOptions['user_id']?.map?.((d) => ({
									id: d,
									label: usersMap.get(d)?.displayName ?? d
								})) ?? []}
								bind:selected={
									() => form.filters.user_id ?? '', (v) => (form.filters.user_id = v ?? '')
								}
								placeholder="user1,user2"
								disabled={isViewMode}
								readonly={isViewMode}
								multiple
							/>

							{#if (isViewMode && form.filters.user_id) || !isViewMode}
								<p class="text-on-surface1 text-xs">List of users</p>
							{/if}
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="mcp_id">Server IDs</label>
							<Select
								class={twMerge(
									'dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner',
									isViewMode && '[color:currentColor] disabled:opacity-100'
								)}
								classes={{
									root: 'w-full',
									clear: 'hover:bg-surface3 bg-transparent'
								}}
								options={filtersOptions['mcp_id']?.map?.((d) => ({ id: d, label: d })) ?? []}
								bind:selected={
									() => form.filters.mcp_id ?? '', (v) => (form.filters.mcp_id = v ?? '')
								}
								placeholder="server1,server2"
								disabled={isViewMode}
								readonly={isViewMode}
								multiple
							/>
							{#if (isViewMode && form.filters.mcp_id) || !isViewMode}
								<p class="text-on-surface1 text-xs">List of server IDs</p>
							{/if}
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="mcp_server_display_name">Server Names</label>
							<Select
								class={twMerge(
									'dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner',
									isViewMode && '[color:currentColor] disabled:opacity-100'
								)}
								classes={{
									root: 'w-full',
									clear: 'hover:bg-surface3 bg-transparent'
								}}
								options={filtersOptions['mcp_server_display_name']?.map?.((d) => ({
									id: d,
									label: d
								})) ?? []}
								bind:selected={
									() => form.filters.mcp_server_display_name ?? '',
									(v) => (form.filters.mcp_server_display_name = v ?? '')
								}
								placeholder="server-name-1,server-name-2"
								disabled={isViewMode}
								readonly={isViewMode}
								multiple
							/>
							{#if (isViewMode && form.filters.mcp_server_display_name) || !isViewMode}
								<p class="text-on-surface1 text-xs">List of server display names</p>
							{/if}
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="call_type">Call Types</label>
							<Select
								class={twMerge(
									'dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner',
									isViewMode && '[color:currentColor] disabled:opacity-100'
								)}
								classes={{
									root: 'w-full',
									clear: 'hover:bg-surface3 bg-transparent'
								}}
								options={filtersOptions['call_type']?.map?.((d) => ({ id: d, label: d })) ?? []}
								bind:selected={
									() => form.filters.call_type ?? '', (v) => (form.filters.call_type = v ?? '')
								}
								placeholder="tools/call,resources/read"
								disabled={isViewMode}
								readonly={isViewMode}
								multiple
							/>
							{#if (isViewMode && form.filters.call_type) || !isViewMode}
								<p class="text-on-surface1 text-xs">List of call types</p>
							{/if}
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="client_name">Client Names</label>
							<Select
								class={twMerge(
									'dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner',
									isViewMode && '[color:currentColor] disabled:opacity-100'
								)}
								classes={{
									root: 'w-full',
									clear: 'hover:bg-surface3 bg-transparent'
								}}
								options={filtersOptions['client_name']?.map?.((d) => ({ id: d, label: d })) ?? []}
								bind:selected={
									() => form.filters.client_name ?? '', (v) => (form.filters.client_name = v ?? '')
								}
								placeholder="client1,client2"
								disabled={isViewMode}
								readonly={isViewMode}
								multiple
							/>
							{#if (isViewMode && form.filters.client_name) || !isViewMode}
								<p class="text-on-surface1 text-xs">List of client names</p>
							{/if}
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="response_status">Response Status</label>
							<Select
								class={twMerge(
									'dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner',
									isViewMode && '[color:currentColor] disabled:opacity-100'
								)}
								classes={{
									root: 'w-full',
									clear: 'hover:bg-surface3 bg-transparent'
								}}
								options={filtersOptions['response_status']?.map?.((d) => ({ id: d, label: d })) ??
									[]}
								bind:selected={
									() => form.filters.response_status ?? '',
									(v) => (form.filters.response_status = v ?? '')
								}
								placeholder="200,400,500"
								disabled={isViewMode}
								readonly={isViewMode}
								multiple
							/>
							{#if (isViewMode && form.filters.response_status) || !isViewMode}
								<p class="text-on-surface1 text-xs">List of HTTP status codes</p>
							{/if}
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="session_id">Session IDs</label>
							<Select
								class={twMerge(
									'dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner',
									isViewMode && '[color:currentColor] disabled:opacity-100'
								)}
								classes={{
									root: 'w-full',
									clear: 'hover:bg-surface3 bg-transparent'
								}}
								options={filtersOptions['session_id']?.map?.((d) => ({ id: d, label: d })) ?? []}
								bind:selected={
									() => form.filters.session_id ?? '', (v) => (form.filters.session_id = v ?? '')
								}
								placeholder="session1,session2"
								disabled={isViewMode}
								readonly={isViewMode}
								multiple
							/>
							{#if (isViewMode && form.filters.session_id) || !isViewMode}
								<p class="text-on-surface1 text-xs">List of session IDs</p>
							{/if}
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="client_ip">Client IPs</label>
							<Select
								class={twMerge(
									'dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner',
									isViewMode && '[color:currentColor] disabled:opacity-100'
								)}
								classes={{
									root: 'w-full',
									clear: 'hover:bg-surface3 bg-transparent'
								}}
								options={filtersOptions['client_ip']?.map?.((d) => ({ id: d, label: d })) ?? []}
								bind:selected={
									() => form.filters.client_ip ?? '', (v) => (form.filters.client_ip = v ?? '')
								}
								placeholder="192.168.1.1,10.0.0.1"
								disabled={isViewMode}
								readonly={isViewMode}
								multiple
							/>
							{#if (isViewMode && form.filters.client_ip) || !isViewMode}
								<p class="text-on-surface1 text-xs">List of IP addresses</p>
							{/if}
						</div>
						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="call_identifier">Call Identifier</label>
							<Select
								class={twMerge(
									'dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner',
									isViewMode && '[color:currentColor] disabled:opacity-100'
								)}
								classes={{
									root: 'w-full',
									clear: 'hover:bg-surface3 bg-transparent'
								}}
								options={filtersOptions['call_identifier']?.map?.((d) => ({ id: d, label: d })) ??
									[]}
								bind:selected={
									() => form.filters.call_identifier ?? '',
									(v) => (form.filters.call_identifier = v ?? '')
								}
								placeholder="call-identifier-1,call-identifier-2"
								disabled={isViewMode}
								readonly={isViewMode}
								multiple
							/>
							{#if (isViewMode && form.filters.call_identifier) || !isViewMode}
								<p class="text-on-surface1 text-xs">List of call identifiers</p>
							{/if}
						</div>
						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="client_version">Client Versions</label>
							<Select
								class={twMerge(
									'dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner',
									isViewMode && '[color:currentColor] disabled:opacity-100'
								)}
								classes={{
									root: 'w-full',
									clear: 'hover:bg-surface3 bg-transparent'
								}}
								options={filtersOptions['client_version']?.map?.((d) => ({ id: d, label: d })) ??
									[]}
								bind:selected={
									() => form.filters.client_version ?? '',
									(v) => (form.filters.client_version = v ?? '')
								}
								placeholder="client-version-1,client-version-2"
								disabled={isViewMode}
								readonly={isViewMode}
								multiple
							/>
							{#if (isViewMode && form.filters.client_version) || !isViewMode}
								<p class="text-on-surface1 text-xs">List of client versions</p>
							{/if}
						</div>
						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="power_user_workspace_id"
								>Catalog Entry Names</label
							>
							<Select
								class={twMerge(
									'dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner',
									isViewMode && '[color:currentColor] disabled:opacity-100'
								)}
								classes={{
									root: 'w-full',
									clear: 'hover:bg-surface3 bg-transparent'
								}}
								options={filtersOptions['mcp_server_catalog_entry_name']?.map?.((d) => ({
									id: d,
									label: d
								})) ?? []}
								bind:selected={
									() => form.filters.mcp_server_catalog_entry_name ?? '',
									(v) => (form.filters.mcp_server_catalog_entry_name = v ?? '')
								}
								placeholder="workspace-id-1,workspace-id-2"
								disabled={isViewMode}
								readonly={isViewMode}
								multiple
							/>
							{#if (isViewMode && form.filters.mcp_server_catalog_entry_name) || !isViewMode}
								<p class="text-on-surface1 text-xs">List of catalog entry names</p>
							{/if}
						</div>
					</div>
				</div>
			{/if}
		</div>

		<!-- Error Display -->
		{#if error}
			<div class="flex items-start gap-3 rounded-md bg-red-50 p-4 dark:bg-red-950/50">
				<AlertTriangle class="size-5 text-red-600 dark:text-red-400" />
				<div class="text-sm text-red-700 dark:text-red-300">
					{error}
				</div>
			</div>
		{/if}

		<!-- Actions -->
		<div class="flex justify-end gap-3 pt-6">
			<button
				type="button"
				class="button"
				onclick={onCancel}
				disabled={creating && mode !== 'view'}
			>
				{mode === 'view' ? 'Back' : 'Cancel'}
			</button>
			{#if mode !== 'view'}
				<button type="submit" class="button-primary" disabled={creating}>
					{#if creating}
						<LoaderCircle class="size-4 animate-spin" />
						{mode === 'edit' ? 'Saving Changes...' : 'Creating Export...'}
					{:else}
						{mode === 'edit' ? 'Save Changes' : 'Create Export'}
					{/if}
				</button>
			{/if}
		</div>
	</form>
</div>
