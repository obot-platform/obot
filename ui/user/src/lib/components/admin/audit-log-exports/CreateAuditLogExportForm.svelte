<script lang="ts">
	import { AdminService, type AuditLogURLFilters } from '$lib/services';
	import { subDays, set } from 'date-fns';
	import { slide } from 'svelte/transition';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import { AlertTriangle, LoaderCircle, ChevronDown, ChevronUp } from 'lucide-svelte';
	import type { DateRange } from '$lib/components/Calendar.svelte';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import type { AuditLogExport } from '$lib/services/admin/types';

	interface Props {
		onCancel: () => void;
		onSubmit: () => void;
		mode?: 'create' | 'view' | 'edit';
		initialData?: AuditLogExport;
	}

	let { onCancel, onSubmit, mode = 'create', initialData }: Props = $props();

	let showAdvancedOptions = $state(false);
	let isViewMode = $derived(mode === 'view');

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

			await AdminService.createAuditLogExport(request);
			onSubmit();
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

<div class="dark:bg-surface2 rounded-md bg-white p-6 shadow-sm">
	<form
		class="space-y-8"
		onsubmit={(e) => {
			e.preventDefault();
			handleSubmit();
		}}
	>
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
						class="text-input-filled"
						id="name"
						bind:value={form.name}
						placeholder="audit-export-2024"
						required={mode !== 'view'}
						readonly={mode === 'view'}
						disabled={isViewMode}
					/>
					{#if (isViewMode && form.name) || !isViewMode}
						<p class="text-xs text-gray-500">Unique name for this export</p>
					{/if}
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
						disabled={isViewMode}
					/>
					{#if (isViewMode && form.bucket) || !isViewMode}
						<p class="text-xs text-gray-500">Storage bucket name where exports will be saved</p>
					{/if}
				</div>
			</div>

			<div class="flex flex-col gap-1">
				<label class="text-sm font-medium" for="keyPrefix">Key Prefix (Optional)</label>
				<input
					class="text-input-filled"
					id="keyPrefix"
					bind:value={form.keyPrefix}
					placeholder="Leave empty for default: mcp-audit-logs/YYYY/MM/DD/"
					readonly={mode === 'view'}
					disabled={isViewMode}
				/>
				{#if (isViewMode && form.keyPrefix) || !isViewMode}
					<p class="text-xs text-gray-500">
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
							<label class="text-sm font-medium" for="user_id">User IDs</label>
							<input
								class={['text-input-filled']}
								id="user_id"
								bind:value={form.filters.user_id}
								placeholder="user1,user2"
								readonly={mode === 'view'}
								disabled={isViewMode}
							/>

							{#if (isViewMode && form.filters.user_id) || !isViewMode}
								<p class="text-xs text-gray-500">Comma-separated user IDs</p>
							{/if}
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="mcp_id">Server IDs</label>
							<input
								class="text-input-filled"
								id="mcp_id"
								bind:value={form.filters.mcp_id}
								placeholder="server1,server2"
								readonly={mode === 'view'}
								disabled={isViewMode}
							/>
							{#if (isViewMode && form.filters.mcp_id) || !isViewMode}
								<p class="text-xs text-gray-500">Comma-separated server IDs</p>
							{/if}
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="mcp_server_display_name">Server Names</label>
							<input
								class="text-input-filled"
								id="mcp_server_display_name"
								bind:value={form.filters.mcp_server_display_name}
								placeholder="server-name-1,server-name-2"
								readonly={mode === 'view'}
								disabled={isViewMode}
							/>
							{#if (isViewMode && form.filters.mcp_server_display_name) || !isViewMode}
								<p class="text-xs text-gray-500">Comma-separated server display names</p>
							{/if}
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="call_type">Call Types</label>
							<input
								class="text-input-filled"
								id="call_type"
								bind:value={form.filters.call_type}
								placeholder="tools/call,resources/read"
								readonly={mode === 'view'}
								disabled={isViewMode}
							/>
							{#if (isViewMode && form.filters.call_type) || !isViewMode}
								<p class="text-xs text-gray-500">Comma-separated call types</p>
							{/if}
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="client_name">Client Names</label>
							<input
								class="text-input-filled"
								id="client_name"
								bind:value={form.filters.client_name}
								placeholder="client1,client2"
								readonly={mode === 'view'}
								disabled={isViewMode}
							/>
							{#if (isViewMode && form.filters.client_name) || !isViewMode}
								<p class="text-xs text-gray-500">Comma-separated client names</p>
							{/if}
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="response_status">Response Status</label>
							<input
								class="text-input-filled"
								id="response_status"
								bind:value={form.filters.response_status}
								placeholder="200,400,500"
								readonly={mode === 'view'}
								disabled={isViewMode}
							/>
							{#if (isViewMode && form.filters.response_status) || !isViewMode}
								<p class="text-xs text-gray-500">Comma-separated HTTP status codes</p>
							{/if}
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="session_id">Session IDs</label>
							<input
								class="text-input-filled"
								id="session_id"
								bind:value={form.filters.session_id}
								placeholder="session1,session2"
								readonly={mode === 'view'}
								disabled={isViewMode}
							/>
							{#if (isViewMode && form.filters.session_id) || !isViewMode}
								<p class="text-xs text-gray-500">Comma-separated session IDs</p>
							{/if}
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="client_ip">Client IPs</label>
							<input
								class="text-input-filled"
								id="client_ip"
								bind:value={form.filters.client_ip}
								placeholder="192.168.1.1,10.0.0.1"
								readonly={mode === 'view'}
								disabled={isViewMode}
							/>
							{#if (isViewMode && form.filters.client_ip) || !isViewMode}
								<p class="text-xs text-gray-500">Comma-separated IP addresses</p>
							{/if}
						</div>
						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="call_identifier">Call Identifier</label>
							<input
								class="text-input-filled"
								id="call_identifier"
								bind:value={form.filters.call_identifier}
								placeholder="call-identifier-1,call-identifier-2"
								readonly={mode === 'view'}
								disabled={isViewMode}
							/>
							{#if (isViewMode && form.filters.call_identifier) || !isViewMode}
								<p class="text-xs text-gray-500">Comma-separated call identifiers</p>
							{/if}
						</div>
						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="client_version">Client Versions</label>
							<input
								class="text-input-filled"
								id="client_version"
								bind:value={form.filters.client_version}
								placeholder="client-version-1,client-version-2"
								readonly={mode === 'view'}
								disabled={isViewMode}
							/>
							{#if (isViewMode && form.filters.client_version) || !isViewMode}
								<p class="text-xs text-gray-500">Comma-separated client versions</p>
							{/if}
						</div>
						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="power_user_workspace_id"
								>Catalog Entry Names</label
							>
							<input
								class="text-input-filled"
								id="power_user_workspace_id"
								bind:value={form.filters.mcp_server_catalog_entry_name}
								placeholder="workspace-id-1,workspace-id-2"
								readonly={mode === 'view'}
								disabled={isViewMode}
							/>
							{#if (isViewMode && form.filters.mcp_server_catalog_entry_name) || !isViewMode}
								<p class="text-xs text-gray-500">Comma-separated catalog entry names</p>
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
