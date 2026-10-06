<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import {
		agentLabel,
		kindLabel,
		PACKAGE_SOURCE_LABELS,
		QUICK_ALLOW_LABELS,
		quickAllowBlockedReason,
		type QuickAllowAction
	} from '$lib/enforcement';
	import { isAbortError } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type EnforcementDecisionAllowlistCheck,
		type EnforcementDecisionEvent
	} from '$lib/services';
	import { userDeviceSettings } from '$lib/stores';
	import { formatLogTimestamp } from '$lib/time';
	import QuickAllowDialog from './QuickAllowDialog.svelte';
	import { CircleAlert, ShieldCheck, X } from '@lucide/svelte';

	interface Props {
		decision: EnforcementDecisionEvent;
		deviceName?: string;
		readOnly?: boolean;
		onClose: () => void;
		onAllowlistUpdated: () => void;
	}

	let {
		decision: initial,
		deviceName,
		readOnly = false,
		onClose,
		onAllowlistUpdated
	}: Props = $props();

	let fetched = $state<EnforcementDecisionEvent>();
	let fetchError = $state<string>();
	let quickAllowDialog = $state<ReturnType<typeof QuickAllowDialog>>();
	let allowlistCheck = $state<EnforcementDecisionAllowlistCheck>();
	let allowlistCheckPending = $state(false);
	let recheckToken = $state(0);

	// Render the row we already have, then replace it with the fetched copy. The
	// list and detail payloads are the same shape, so a failed refresh costs
	// nothing beyond a note.
	let decision = $derived(fetched ?? initial);

	const quickAllowActions: QuickAllowAction[] = ['hostname', 'server', 'tool'];

	let quickAllows = $derived(
		quickAllowActions.map((action) => ({
			action,
			blocked: quickAllowBlockedReason(decision, action)
		}))
	);
	let canQuickAllow = $derived(quickAllows.some(({ blocked }) => !blocked));

	let checkable = $derived(
		initial.decision === 'deny' &&
			quickAllowActions.some((action) => !quickAllowBlockedReason(initial, action))
	);
	let checkRequest = $derived(checkable ? { id: initial.id, token: recheckToken } : undefined);
	let alreadyAllowed = $derived(allowlistCheck?.allowlistDecision === 'allow');
	let checkingAllowlist = $derived(checkable && allowlistCheckPending);

	$effect(() => {
		const id = initial.id;
		if (!id) return;

		const controller = new AbortController();
		fetched = undefined;
		fetchError = undefined;

		AdminService.getEnforcementDecision(id, { signal: controller.signal })
			.then((response) => {
				if (controller.signal.aborted) return;
				fetched = response;
			})
			.catch((err) => {
				if (isAbortError(err) || controller.signal.aborted) return;
				fetchError =
					err instanceof Error
						? err.message
						: m.inventory_enforcement_enforcement_events_load_details_failed();
			});

		return () => controller.abort();
	});

	// Ask the backend whether the current allowlist already covers this call.
	$effect(() => {
		const request = checkRequest;
		if (!request?.id) {
			allowlistCheck = undefined;
			allowlistCheckPending = false;
			return;
		}

		const controller = new AbortController();
		allowlistCheck = undefined;
		allowlistCheckPending = true;

		AdminService.checkEnforcementDecisionAllowlist(request.id, { signal: controller.signal })
			.then((response) => {
				if (controller.signal.aborted) return;
				allowlistCheck = response;
			})
			.catch((err) => {
				if (isAbortError(err) || controller.signal.aborted) return;
			})
			.finally(() => {
				if (controller.signal.aborted) return;
				allowlistCheckPending = false;
			});

		return () => controller.abort();
	});
</script>

<div class="bg-base-200 text-base-content flex h-full w-[inherit] min-w-[inherit] flex-col">
	<div
		class="dark:bg-base-200 bg-base-100 relative flex w-full items-center justify-between p-4 pl-5 shadow-xs"
	>
		<div class="bg-primary absolute top-0 left-0 h-full w-1"></div>
		<h3 class="text-lg font-semibold">
			{m.inventory_enforcement_enforcement_events_decision_detail()}
		</h3>
		<IconButton onclick={onClose}>
			<X class="size-5" />
		</IconButton>
	</div>

	<div class="default-scrollbar-thin relative flex-1 overflow-y-auto pb-4">
		<div class="bg-base-300 absolute top-0 left-0 h-full w-1"></div>

		<div class="flex flex-col gap-1 p-4 pl-5">
			<div class="flex flex-wrap items-center gap-2">
				{#if decision.decision === 'allow'}
					<span class="badge badge-success badge-sm"
						>{m.inventory_enforcement_enforcement_events_allowed()}</span
					>
				{:else}
					<span class="badge badge-error badge-sm"
						>{m.inventory_enforcement_enforcement_events_blocked()}</span
					>
				{/if}
				{#if decision.unresolved}
					<span class="badge badge-warning badge-sm"
						>{m.inventory_enforcement_enforcement_events_unidentified()}</span
					>
				{/if}
				{#if decision.obotHosted}
					<span class="badge badge-ghost badge-sm"
						>{m.inventory_enforcement_enforcement_events_obot_hosted()}</span
					>
				{/if}
				<span class="text-muted-content text-xs">
					{formatLogTimestamp(decision.createdAt, userDeviceSettings.timeFormat)}
				</span>
			</div>
			{#if decision.unresolvedReason || decision.reason}
				<p class="text-muted-content text-sm font-light">
					{decision.unresolvedReason || decision.reason}
				</p>
			{/if}
		</div>

		{#if fetchError}
			<div class="notification-alert mx-4 mb-2 ml-5 flex items-start gap-2.5 p-2.5">
				<CircleAlert class="size-4 shrink-0" />
				<span class="text-xs break-all">
					{m.inventory_enforcement_enforcement_events_showing_summary({ error: fetchError })}
				</span>
			</div>
		{/if}

		<div class="flex flex-col gap-6 p-4 pl-5">
			<div class="flex flex-col gap-1.5">
				<p class="text-base font-semibold">{m.inventory_enforcement_enforcement_events_call()}</p>
				<div class="grid grid-cols-[9rem_1fr] gap-x-2 gap-y-1 text-sm font-light">
					<span class="font-medium">{m.audit_usage_audit_logs_agent()}</span>
					<span>{agentLabel(decision.agent)}</span>
					<span class="font-medium">{m.inventory_enforcement_enforcement_events_tool()}</span>
					<span class="break-all">{decision.tool || '—'}</span>
					<span class="font-medium">{m.inventory_enforcement_enforcement_events_tool_type()}</span>
					<span>{kindLabel(decision.kind)}</span>
					<span class="font-medium">{m.inventory_enforcement_mcp_server()}</span>
					<span class="break-all">{decision.serverName || '—'}</span>
				</div>
			</div>

			{#if decision.server}
				{@const server = decision.server}
				<div class="flex flex-col gap-1.5">
					<p class="text-base font-semibold">
						{m.inventory_enforcement_enforcement_events_resolved_target()}
					</p>
					<div class="grid grid-cols-[9rem_1fr] gap-x-2 gap-y-1 text-sm font-light">
						{#if server.url}
							<span class="font-medium">URL</span>
							<span class="break-all">{server.url}</span>
						{/if}
						{#if server.hostname}
							<span class="font-medium">{m.core_col_hostname()}</span>
							<span class="break-all">{server.hostname}</span>
						{/if}
						{#if server.package}
							<span class="font-medium">{m.core_col_registry()}</span>
							<span>{PACKAGE_SOURCE_LABELS[server.package.source] ?? server.package.source}</span>
							<span class="font-medium">{m.inventory_enforcement_enforcement_events_package()}</span
							>
							<span class="break-all">{server.package.name}</span>
							<span class="font-medium">{m.inventory_enforcement_enforcement_events_version()}</span
							>
							<span class="break-all"
								>{server.package.version ||
									m.inventory_enforcement_enforcement_events_not_reported()}</span
							>
						{/if}
						{#if server.connector}
							<span class="font-medium"
								>{m.inventory_enforcement_enforcement_events_connector()}</span
							>
							<span class="break-all">{server.connector}</span>
						{/if}
						{#if server.command}
							<span class="font-medium">{m.inventory_enforcement_enforcement_events_command()}</span
							>
							<span class="break-all">{server.command}</span>
						{/if}
					</div>
				</div>
			{/if}

			<div class="flex flex-col gap-1.5">
				<p class="text-base font-semibold">{m.audit_usage_audit_logs_device()}</p>
				<div class="grid grid-cols-[9rem_1fr] gap-x-2 gap-y-1 text-sm font-light">
					{#if deviceName && deviceName !== decision.deviceID}
						<span class="font-medium">{m.core_col_hostname()}</span>
						<span class="break-all">{deviceName}</span>
					{/if}
					<span class="font-medium">{m.inventory_enforcement_enforcement_events_device_id()}</span>
					<span class="break-all">{decision.deviceID || '—'}</span>
					<span class="font-medium">{m.audit_usage_audit_logs_model_col_ip_address()}</span>
					<span class="break-all">{decision.clientIP || '—'}</span>
					<span class="font-medium">{m.inventory_enforcement_configuration_tab()}</span>
					<span>#{decision.mdmConfigurationID}</span>
				</div>
			</div>

			{#if decision.decision === 'deny' && canQuickAllow}
				<div class="flex flex-col gap-2">
					{#if checkingAllowlist}
						<p class="text-base font-semibold">
							{m.inventory_enforcement_enforcement_events_allow_going_forward()}
						</p>
						<div class="text-muted-content flex items-center gap-2 text-sm font-light">
							<Loading class="size-4" />
							<span>{m.inventory_enforcement_enforcement_events_checking_allowlist()}</span>
						</div>
					{:else if alreadyAllowed}
						<p class="text-base font-semibold">
							{m.inventory_enforcement_enforcement_events_already_allowed()}
						</p>
						<div class="notification-info flex items-start gap-2.5 p-2.5">
							<ShieldCheck class="size-4 shrink-0" />
							<div class="flex flex-col gap-1">
								<span class="text-xs">
									{m.inventory_enforcement_enforcement_events_already_allowed_note()}
								</span>
								{#if allowlistCheck?.allowlistReason}
									<span class="text-xs font-light wrap-break-word">
										{allowlistCheck.allowlistReason}
									</span>
								{/if}
							</div>
						</div>
					{:else}
						<p class="text-base font-semibold">
							{m.inventory_enforcement_enforcement_events_allow_going_forward()}
						</p>
						{#if readOnly}
							<p class="text-muted-content text-sm font-light">
								{m.inventory_enforcement_enforcement_events_requires_write_access()}
							</p>
						{:else}
							<p class="text-muted-content text-sm font-light">
								{m.inventory_enforcement_enforcement_events_adds_rule()}
							</p>
							<div class="flex flex-col gap-2">
								{#each quickAllows as { action, blocked } (action)}
									<span use:tooltip={blocked ?? undefined} class="flex w-full max-w-sm">
										<button
											class="btn btn-secondary hover:bg-primary hover:text-primary-content w-full justify-center text-center disabled:opacity-50"
											disabled={Boolean(blocked)}
											onclick={() => quickAllowDialog?.open(decision, action)}
										>
											{QUICK_ALLOW_LABELS[action]}
										</button>
									</span>
									{#if blocked}
										<p class="text-muted-content -mt-1 text-xs font-light wrap-break-word">
											{blocked}
										</p>
									{/if}
								{/each}
							</div>
						{/if}
					{/if}
				</div>
			{/if}
		</div>
	</div>
</div>

<QuickAllowDialog
	bind:this={quickAllowDialog}
	onApplied={() => {
		// Re-ask the server so this panel reflects the rule that was just added.
		recheckToken += 1;
		onAllowlistUpdated();
	}}
/>
