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
	import { AdminService, type EnforcementDecisionEvent } from '$lib/services';
	import { userDeviceSettings } from '$lib/stores';
	import { formatLogTimestamp } from '$lib/time';
	import QuickAllowDialog from './QuickAllowDialog.svelte';
	import { CircleAlert, X } from '@lucide/svelte';

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

	// Render the row we already have, then replace it with the fetched copy. The
	// list and detail payloads are the same shape, so a failed refresh costs
	// nothing beyond a note.
	let decision = $derived(fetched ?? initial);

	const quickAllowActions: QuickAllowAction[] = ['hostname', 'server', 'tool'];

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
				fetchError = err instanceof Error ? err.message : 'Failed to load decision details';
			});

		return () => controller.abort();
	});
</script>

<div class="bg-base-200 text-base-content flex h-full w-[inherit] min-w-[inherit] flex-col">
	<div
		class="dark:bg-base-200 bg-base-100 relative flex w-full items-center justify-between p-4 pl-5 shadow-xs"
	>
		<div class="bg-primary absolute top-0 left-0 h-full w-1"></div>
		<h3 class="text-lg font-semibold">Decision Detail</h3>
		<IconButton onclick={onClose}>
			<X class="size-5" />
		</IconButton>
	</div>

	<div class="default-scrollbar-thin relative flex-1 overflow-y-auto pb-4">
		<div class="bg-base-300 absolute top-0 left-0 h-full w-1"></div>

		<div class="flex flex-col gap-1 p-4 pl-5">
			<div class="flex flex-wrap items-center gap-2">
				{#if decision.decision === 'allow'}
					<span class="badge badge-success badge-sm">Allowed</span>
				{:else}
					<span class="badge badge-error badge-sm">Blocked</span>
				{/if}
				{#if decision.unresolved}
					<span class="badge badge-warning badge-sm">Could not be identified</span>
				{/if}
				{#if decision.obotHosted}
					<span class="badge badge-ghost badge-sm">Obot-hosted</span>
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
					Showing the summary from the list — the full record couldn't be loaded. {fetchError}
				</span>
			</div>
		{/if}

		<div class="flex flex-col gap-6 p-4 pl-5">
			<div class="flex flex-col gap-1.5">
				<p class="text-base font-semibold">Call</p>
				<div class="grid grid-cols-[9rem_1fr] gap-x-2 gap-y-1 text-sm font-light">
					<span class="font-medium">Agent</span>
					<span>{agentLabel(decision.agent)}</span>
					<span class="font-medium">Tool</span>
					<span class="break-all">{decision.tool || '—'}</span>
					<span class="font-medium">Tool Type</span>
					<span>{kindLabel(decision.kind)}</span>
					<span class="font-medium">MCP Server</span>
					<span class="break-all">{decision.serverName || '—'}</span>
				</div>
			</div>

			{#if decision.server}
				{@const server = decision.server}
				<div class="flex flex-col gap-1.5">
					<p class="text-base font-semibold">Resolved Target</p>
					<div class="grid grid-cols-[9rem_1fr] gap-x-2 gap-y-1 text-sm font-light">
						{#if server.url}
							<span class="font-medium">URL</span>
							<span class="break-all">{server.url}</span>
						{/if}
						{#if server.hostname}
							<span class="font-medium">Hostname</span>
							<span class="break-all">{server.hostname}</span>
						{/if}
						{#if server.package}
							<span class="font-medium">Registry</span>
							<span>{PACKAGE_SOURCE_LABELS[server.package.source] ?? server.package.source}</span>
							<span class="font-medium">Package</span>
							<span class="break-all">{server.package.name}</span>
							<span class="font-medium">Version</span>
							<span class="break-all">{server.package.version || 'Not reported'}</span>
						{/if}
						{#if server.connector}
							<span class="font-medium">Connector</span>
							<span class="break-all">{server.connector}</span>
						{/if}
						{#if server.command}
							<span class="font-medium">Command</span>
							<span class="break-all">{server.command}</span>
						{/if}
					</div>
				</div>
			{/if}

			<div class="flex flex-col gap-1.5">
				<p class="text-base font-semibold">Device</p>
				<div class="grid grid-cols-[9rem_1fr] gap-x-2 gap-y-1 text-sm font-light">
					{#if deviceName && deviceName !== decision.deviceID}
						<span class="font-medium">Hostname</span>
						<span class="break-all">{deviceName}</span>
					{/if}
					<span class="font-medium">Device ID</span>
					<span class="break-all">{decision.deviceID || '—'}</span>
					<span class="font-medium">IP Address</span>
					<span class="break-all">{decision.clientIP || '—'}</span>
					<span class="font-medium">Configuration</span>
					<span>#{decision.mdmConfigurationID}</span>
				</div>
			</div>

			{#if decision.decision === 'deny'}
				<div class="flex flex-col gap-2">
					<p class="text-base font-semibold">Allow this call going forward</p>
					{#if readOnly}
						<p class="text-muted-content text-sm font-light">
							Requires an administrator with write access.
						</p>
					{:else}
						<p class="text-muted-content text-sm font-light">
							Adds a rule to this fleet's enforcement allowlist.
						</p>
						<div class="flex flex-col gap-2">
							{#each quickAllowActions as action (action)}
								{@const blocked = quickAllowBlockedReason(decision, action)}
								<span use:tooltip={blocked ?? undefined} class="flex">
									<button
										class="btn btn-secondary w-full justify-start text-left text-sm"
										disabled={Boolean(blocked)}
										onclick={() => quickAllowDialog?.open(decision, action)}
									>
										{QUICK_ALLOW_LABELS[action]}
									</button>
								</span>
								{#if blocked}
									<p class="text-muted-content -mt-1 text-xs font-light break-all">{blocked}</p>
								{/if}
							{/each}
						</div>
					{/if}
				</div>
			{/if}
		</div>
	</div>
</div>

<QuickAllowDialog bind:this={quickAllowDialog} onApplied={onAllowlistUpdated} />
