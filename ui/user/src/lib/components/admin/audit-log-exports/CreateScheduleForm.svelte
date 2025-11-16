<script lang="ts">
	import { AdminService, type AuditLogURLFilters } from '$lib/services';
	import { slide } from 'svelte/transition';
	import Dropdown from '$lib/components/tasks/Dropdown.svelte';
	import { AlertTriangle, LoaderCircle, GlobeIcon, ChevronDown, ChevronUp } from 'lucide-svelte';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import type { ScheduledAuditLogExport } from '$lib/services/admin/types';

	interface Props {
		onCancel: () => void;
		onSubmit: (result?: ScheduledAuditLogExport) => void;
		mode?: 'create' | 'view' | 'edit';
		initialData?: ScheduledAuditLogExport;
	}

	let { onCancel, onSubmit, mode = 'create', initialData }: Props = $props();

	let defaultTimezone = $state(Intl.DateTimeFormat().resolvedOptions().timeZone);
	let showAdvancedOptions = $state(false);
	// Form state
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

	// Populate form from URL parameters (from audit logs page) or initialData
	onMount(async () => {
		if (initialData && (mode === 'view' || mode === 'edit')) {
			// Populate from initialData for view/edit modes
			form.name = initialData.name || '';
			form.enabled = initialData.enabled !== undefined ? initialData.enabled : true;
			form.bucket = initialData.bucket || '';
			form.keyPrefix = initialData.keyPrefix || '';
			form.retentionPeriodInDays = initialData.retentionPeriodInDays || 30;

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
			if (initialData.filters) {
				form.filters = {
					user_id: initialData.filters.userIDs ? initialData.filters.userIDs.join(',') : '',
					mcp_id: initialData.filters.mcpIDs ? initialData.filters.mcpIDs.join(',') : '',
					mcp_server_display_name: initialData.filters.mcpServerDisplayNames
						? initialData.filters.mcpServerDisplayNames.join(',')
						: '',
					mcp_server_catalog_entry_name: initialData.filters.mcpServerCatalogEntryNames
						? initialData.filters.mcpServerCatalogEntryNames.join(',')
						: '',
					call_type: initialData.filters.callTypes ? initialData.filters.callTypes.join(',') : '',
					call_identifier: initialData.filters.callIdentifiers
						? initialData.filters.callIdentifiers.join(',')
						: '',
					response_status: initialData.filters.responseStatuses
						? initialData.filters.responseStatuses.join(',')
						: '',
					session_id: initialData.filters.sessionIDs
						? initialData.filters.sessionIDs.join(',')
						: '',
					client_name: initialData.filters.clientNames
						? initialData.filters.clientNames.join(',')
						: '',
					client_version: initialData.filters.clientVersions
						? initialData.filters.clientVersions.join(',')
						: '',
					client_ip: initialData.filters.clientIPs ? initialData.filters.clientIPs.join(',') : ''
				};
				showAdvancedOptions = true;
			}
		} else if (mode === 'create') {
			// Populate from URL parameters for create mode
			const params = page.url.searchParams;

			const mappedField = {
				user_ids: 'user_id',
				mcp_ids: 'mcp_id',
				mcp_server_display_names: 'mcp_server_display_name',
				mcp_server_catalog_entry_names: 'mcp_server_catalog_entry_name',
				call_types: 'call_type',
				call_identifiers: 'call_identifier',
				response_statuses: 'response_status',
				session_ids: 'session_id',
				client_names: 'client_name',
				client_versions: 'client_version',
				client_ips: 'client_ip'
			} satisfies Record<string, keyof AuditLogURLFilters>;

			let hasFilters = false;
			for (const [key, value] of Object.entries(mappedField)) {
				if (params.get(key)) {
					form.filters[value] = params.get(key);
					hasFilters = true;
				}
			}

			// Show advanced options if there are filters from the URL
			if (hasFilters) {
				showAdvancedOptions = true;
			}
		}
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

			// Prepare the request
			const request = {
				name: form.name,
				bucket: form.bucket,
				keyPrefix: form.keyPrefix,
				enabled: form.enabled,
				schedule: form.schedule,
				retentionPeriodInDays: form.retentionPeriodInDays,
				filters: {
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
						: []
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
				})) as typeof result;
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
					<p class="text-xs text-gray-500">Unique name for this export schedule</p>
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
					<p class="text-xs text-gray-500">Storage bucket name where exports will be saved</p>
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
				/>
				<p class="text-xs text-gray-500">
					Path prefix within the bucket. If empty, defaults to "mcp-audit-logs/YYYY/MM/DD/" format
					based on current date.
				</p>
			</div>
		</div>

		<!-- Schedule Configuration -->
		<div class="flex flex-col gap-4">
			<h3 class="text-lg font-semibold">Schedule Configuration</h3>

			<div class="flex w-[50%] flex-wrap gap-4">
				<Dropdown
					class="schedule-dropdown"
					values={{
						hourly: 'hourly',
						daily: 'daily',
						weekly: 'weekly',
						monthly: 'monthly'
					}}
					selected={form.schedule.interval}
					onSelected={(value) => {
						if (mode !== 'view') {
							form.schedule.interval = value;
						}
					}}
					disabled={mode === 'view'}
				/>

				{#if form.schedule.interval === 'hourly'}
					<Dropdown
						class="schedule-dropdown"
						values={{
							'0': 'on the hour',
							'15': '15 minutes past',
							'30': '30 minutes past',
							'45': '45 minutes past'
						}}
						selected={form.schedule.minute.toString()}
						onSelected={(value) => {
							form.schedule.minute = parseInt(value);
						}}
					/>
				{/if}

				{#if form.schedule.interval === 'daily'}
					<Dropdown
						class="schedule-dropdown"
						values={{
							'0': 'midnight',
							'3': '3 AM',
							'6': '6 AM',
							'9': '9 AM',
							'12': 'noon',
							'15': '3 PM',
							'18': '6 PM',
							'21': '9 PM'
						}}
						selected={form.schedule.hour.toString()}
						onSelected={(value) => {
							form.schedule.hour = parseInt(value);
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
					<Dropdown
						class="schedule-dropdown"
						values={{
							'0': 'Sunday',
							'1': 'Monday',
							'2': 'Tuesday',
							'3': 'Wednesday',
							'4': 'Thursday',
							'5': 'Friday',
							'6': 'Saturday'
						}}
						selected={form.schedule.weekday.toString()}
						onSelected={(value) => {
							form.schedule.weekday = parseInt(value);
						}}
					/>
					<Dropdown
						class="schedule-dropdown"
						values={{
							'0': 'midnight',
							'3': '3 AM',
							'6': '6 AM',
							'9': '9 AM',
							'12': 'noon',
							'15': '3 PM',
							'18': '6 PM',
							'21': '9 PM'
						}}
						selected={form.schedule.hour.toString()}
						onSelected={(value) => {
							form.schedule.hour = parseInt(value);
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
					<Dropdown
						class="schedule-dropdown"
						values={{
							'0': '1st',
							'1': '2nd',
							'2': '3rd',
							'4': '5th',
							'14': '15th',
							'19': '20th',
							'24': '25th',
							'-1': 'last day'
						}}
						selected={form.schedule.day.toString()}
						onSelected={(value) => {
							form.schedule.day = parseInt(value);
						}}
					/>
					<Dropdown
						class="schedule-dropdown"
						values={{
							'0': 'midnight',
							'3': '3 AM',
							'6': '6 AM',
							'9': '9 AM',
							'12': 'noon',
							'15': '3 PM',
							'18': '6 PM',
							'21': '9 PM'
						}}
						selected={form.schedule.hour.toString()}
						onSelected={(value) => {
							form.schedule.hour = parseInt(value);
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
				<Dropdown
					class="schedule-dropdown w-full max-w-xs"
					values={{
						'1': 'Last 1 day',
						'3': 'Last 3 days',
						'7': 'Last 7 days',
						'30': 'Last 30 days',
						'60': 'Last 60 days',
						'90': 'Last 90 days',
						'-1': 'All logs'
					}}
					selected={form.retentionPeriodInDays.toString()}
					onSelected={(value) => {
						form.retentionPeriodInDays = parseInt(value);
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

					<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="user_id">User IDs</label>
							<input
								class="text-input-filled"
								id="user_id"
								bind:value={form.filters.user_id}
								placeholder="user1,user2"
								readonly={mode === 'view'}
							/>
							<p class="text-xs text-gray-500">Comma-separated user IDs</p>
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="mcp_id">Server IDs</label>
							<input
								class="text-input-filled"
								id="mcp_id"
								bind:value={form.filters.mcp_id}
								placeholder="server1,server2"
								readonly={mode === 'view'}
							/>
							<p class="text-xs text-gray-500">Comma-separated server IDs</p>
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="mcp_server_display_name">Server Names</label>
							<input
								class="text-input-filled"
								id="mcp_server_display_name"
								bind:value={form.filters.mcp_server_display_name}
								placeholder="server-name-1,server-name-2"
								readonly={mode === 'view'}
							/>
							<p class="text-xs text-gray-500">Comma-separated server display names</p>
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="call_type">Call Types</label>
							<input
								class="text-input-filled"
								id="call_type"
								bind:value={form.filters.call_type}
								placeholder="tools/call,resources/read"
								readonly={mode === 'view'}
							/>
							<p class="text-xs text-gray-500">Comma-separated call types</p>
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="client_name">Client Names</label>
							<input
								class="text-input-filled"
								id="client_name"
								bind:value={form.filters.client_name}
								placeholder="client1,client2"
								readonly={mode === 'view'}
							/>
							<p class="text-xs text-gray-500">Comma-separated client names</p>
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="response_status">Response Status</label>
							<input
								class="text-input-filled"
								id="response_status"
								bind:value={form.filters.response_status}
								placeholder="200,400,500"
								readonly={mode === 'view'}
							/>
							<p class="text-xs text-gray-500">Comma-separated HTTP status codes</p>
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="session_id">Session IDs</label>
							<input
								class="text-input-filled"
								id="session_id"
								bind:value={form.filters.session_id}
								placeholder="session1,session2"
								readonly={mode === 'view'}
							/>
							<p class="text-xs text-gray-500">Comma-separated session IDs</p>
						</div>

						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="client_ip">Client IPs</label>
							<input
								class="text-input-filled"
								id="client_ip"
								bind:value={form.filters.client_ip}
								placeholder="192.168.1.1,10.0.0.1"
								readonly={mode === 'view'}
							/>
							<p class="text-xs text-gray-500">Comma-separated IP addresses</p>
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
							/>
							<p class="text-xs text-gray-500">Comma-separated catalog entry names</p>
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
						{mode === 'edit' ? 'Saving Changes...' : 'Creating Schedule...'}
					{:else}
						{mode === 'edit' ? 'Save Changes' : 'Create Schedule'}
					{/if}
				</button>
			{/if}
		</div>
	</form>
</div>

<style lang="postcss">
	:global(.schedule-dropdown) {
		background-color: var(--surface2);
		font-size: var(--text-md);
		display: flex;
		flex-grow: 1;
	}
</style>
