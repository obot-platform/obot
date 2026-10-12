<script lang="ts">
	import { invalidate } from '$app/navigation';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import {
		allowlistServerKind,
		mergeAllowlistEntry,
		PACKAGE_SOURCE_LABELS,
		QUICK_ALLOW_LABELS,
		quickAllowEntry,
		type MergeEffect,
		type QuickAllowAction
	} from '$lib/enforcement';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type AllowlistServer,
		type EnforcementDecisionEvent,
		type MDMConfiguration
	} from '$lib/services';
	import { CircleAlert, TriangleAlert } from '@lucide/svelte';

	interface Props {
		onApplied: () => void;
	}

	let { onApplied }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let action = $state<QuickAllowAction>('server');
	let entry = $state<AllowlistServer>();
	let configuration = $state<MDMConfiguration>();
	let effect = $state<MergeEffect>();
	let loading = $state(false);
	let saving = $state(false);
	let loadError = $state<string>();
	let saveError = $state<string>();
	let applied = $state(false);

	let kind = $derived(entry ? allowlistServerKind(entry) : undefined);

	// The dialog refetches the configuration on open rather than trusting the copy
	// the page loaded, because the enforcement endpoint replaces the whole policy
	// and composing that from stale state would silently drop another
	// administrator's edits.
	export async function open(decision: EnforcementDecisionEvent, next: QuickAllowAction) {
		action = next;
		entry = quickAllowEntry(decision, next);
		configuration = undefined;
		effect = undefined;
		loadError = undefined;
		saveError = undefined;
		applied = false;
		if (!entry) return;

		dialog?.open();
		loading = true;
		try {
			const current = await AdminService.getMDMConfiguration(decision.mdmConfigurationID);
			configuration = current;
			effect = mergeAllowlistEntry(current.enforcementAllowlist ?? {}, entry).effect;
		} catch (error) {
			loadError = parseErrorContent(error).message;
		} finally {
			loading = false;
		}
	}

	async function apply() {
		if (!entry || !configuration) return;
		saving = true;
		saveError = undefined;
		try {
			const merged = mergeAllowlistEntry(configuration.enforcementAllowlist ?? {}, entry);
			await AdminService.updateMDMConfigurationEnforcement(configuration.id, {
				// Approving one call must never flip enforcement on or off.
				enforcementEnabled: configuration.enforcementEnabled ?? false,
				enforcementAllowlist: merged.allowlist
			});
			applied = true;
			onApplied();
			// The devices page loads this same policy in its own load function, so its
			// data is now stale.
			await invalidate('devices:data');
		} catch (error) {
			saveError = parseErrorContent(error).message;
		} finally {
			saving = false;
		}
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	title={m.inventory_enforcement_enforcement_events_add_to_allowlist()}
	class="w-full max-w-md"
>
	{#if loading}
		<div class="text-muted-content flex items-center justify-center gap-2 py-8 text-sm">
			<Loading class="size-5" />
			<span>{m.inventory_enforcement_enforcement_events_loading_policy()}</span>
		</div>
	{:else if loadError}
		<div class="notification-error flex items-start gap-3 p-3">
			<CircleAlert class="size-4 shrink-0" />
			<div class="flex flex-col gap-1">
				<p class="text-sm font-semibold">
					{m.inventory_enforcement_enforcement_events_unable_to_load_policy()}
				</p>
				<p class="text-sm font-light break-all">{loadError}</p>
			</div>
		</div>
	{:else if applied}
		<p class="text-sm">{m.inventory_enforcement_enforcement_events_allowlist_updated()}</p>
	{:else if entry}
		<div class="flex flex-col gap-4">
			<p class="text-sm font-medium">{QUICK_ALLOW_LABELS[action]}</p>

			<div class="bg-base-200 dark:bg-base-300 flex flex-col gap-1.5 rounded-lg p-3 text-sm">
				{#if kind === 'url'}
					<div class="grid grid-cols-[7rem_1fr] gap-2">
						<span class="font-medium">URL</span>
						<span class="break-all">{entry.url}</span>
					</div>
				{:else if kind === 'hostname'}
					<div class="grid grid-cols-[7rem_1fr] gap-2">
						<span class="font-medium">{m.core_col_hostname()}</span>
						<span class="break-all">{entry.hostname}</span>
					</div>
				{:else if kind === 'connector'}
					<div class="grid grid-cols-[7rem_1fr] gap-2">
						<span class="font-medium">{m.inventory_enforcement_enforcement_events_connector()}</span
						>
						<span class="break-all">{entry.connector}</span>
					</div>
				{:else if kind === 'package'}
					<div class="grid grid-cols-[7rem_1fr] gap-2">
						<span class="font-medium">{m.core_col_registry()}</span>
						<span>{PACKAGE_SOURCE_LABELS[entry.package!.source]}</span>
					</div>
					<div class="grid grid-cols-[7rem_1fr] gap-2">
						<span class="font-medium">{m.inventory_enforcement_enforcement_events_package()}</span>
						<span class="break-all">{entry.package!.name}</span>
					</div>
					<div class="grid grid-cols-[7rem_1fr] gap-2">
						<span class="font-medium">{m.inventory_enforcement_enforcement_events_version()}</span>
						<span>{m.inventory_enforcement_enforcement_events_any()}</span>
					</div>
				{/if}
				<div class="grid grid-cols-[7rem_1fr] gap-2">
					<span class="font-medium">{m.audit_usage_exports_filter_title_tools()}</span>
					<span class="break-all"
						>{entry.tools?.length
							? entry.tools.join(', ')
							: m.inventory_enforcement_enforcement_events_all()}</span
					>
				</div>
			</div>

			{#if effect === 'no-op'}
				<p class="text-muted-content text-sm">
					{m.inventory_enforcement_enforcement_events_allowlist_covers_note()}
				</p>
			{:else}
				<p class="text-muted-content text-sm">
					{#if action === 'hostname'}
						{m.inventory_enforcement_enforcement_events_future_hostname()}
					{:else if action === 'server'}
						{m.inventory_enforcement_enforcement_events_future_server()}
					{:else}
						{m.inventory_enforcement_enforcement_events_future_tool()}
					{/if}
				</p>

				{#if effect === 'widened'}
					<p class="text-muted-content text-xs">
						{m.inventory_enforcement_enforcement_events_widened_note()}
					</p>
				{:else if effect === 'tool-added'}
					<p class="text-muted-content text-xs">
						{m.inventory_enforcement_enforcement_events_tool_added_note()}
					</p>
				{/if}

				{#if configuration && !configuration.enforcementEnabled}
					<div class="notification-alert flex items-start gap-2.5 p-2.5">
						<TriangleAlert class="size-4 shrink-0" />
						<span class="text-xs">
							{m.inventory_enforcement_enforcement_events_disabled_rule_note()}
						</span>
					</div>
				{/if}
			{/if}

			{#if saveError}
				<p class="text-error text-xs break-all">{saveError}</p>
			{/if}
		</div>
	{/if}

	<div class="mt-6 flex justify-end gap-2">
		{#if applied || loadError || effect === 'no-op'}
			<button class="btn btn-primary" onclick={() => dialog?.close()}>{m.core_close()}</button>
		{:else}
			<button class="btn btn-secondary" disabled={saving} onclick={() => dialog?.close()}>
				{m.common_cancel()}
			</button>
			<button
				class="btn btn-primary flex items-center gap-2"
				disabled={saving || loading || !entry || !configuration}
				onclick={apply}
			>
				{#if saving}<Loading class="size-4" />{/if}
				{m.inventory_enforcement_enforcement_events_add_to_allowlist()}
			</button>
		{/if}
	</div>
</ResponsiveDialog>
