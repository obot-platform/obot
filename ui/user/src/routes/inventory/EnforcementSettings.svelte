<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { MDM_DEVICES_CONFIGURATION_FIELD_IDS } from '$lib/constants';
	import {
		ALLOWLIST_SERVER_KIND_LABELS,
		allowlistServerKind,
		allowlistServerLabel,
		canonicalAllowlist,
		defaultAllowlist,
		isAllowlistEmpty,
		mergeAllowlistEntry
	} from '$lib/enforcement';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type AllowlistServer,
		type EnforcementAllowlist,
		type MDMConfiguration
	} from '$lib/services';
	import AllowlistServerDialog from './AllowlistServerDialog.svelte';
	import {
		ChevronDown,
		Pencil,
		Plus,
		Save,
		ShieldCheck,
		Trash2,
		TriangleAlert
	} from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { slide } from 'svelte/transition';

	interface Props {
		configuration: MDMConfiguration;
		readOnly?: boolean;
		onUpdate: (configuration: MDMConfiguration) => void;
	}

	let { configuration: givenConfiguration, readOnly = false, onUpdate }: Props = $props();

	function cloneAllowlist(source?: EnforcementAllowlist): EnforcementAllowlist {
		return source ? $state.snapshot(source) : {};
	}

	let configuration = $state(untrack(() => givenConfiguration));
	let enabled = $state(untrack(() => givenConfiguration.enforcementEnabled ?? false));
	// The configuration arrives as a reactive proxy from the page, so it is
	// snapshotted rather than cloned outright — the form must own plain data it can
	// mutate freely without writing through to the page's copy.
	let allowlist = $state(untrack(() => cloneAllowlist(givenConfiguration.enforcementAllowlist)));
	let saving = $state(false);
	let operationError = $state<string>();
	let errorIndex = $state<number>();
	let seededNote = $state(false);
	let confirmEmpty = $state(false);
	let removingIndex = $state<number>();
	let serverDialog = $state<ReturnType<typeof AllowlistServerDialog>>();
	let serversOpen = $state(
		untrack(() => (givenConfiguration.enforcementAllowlist?.servers?.length ?? 0) > 0)
	);
	// The rules only matter while enforcement is on, so they start collapsed when it
	// is off. An administrator can still expand them to stage a policy first.
	let rulesOpen = $state(untrack(() => givenConfiguration.enforcementEnabled ?? false));

	// A fresh configuration can arrive while this form is mounted — saving here, or
	// an allowlist entry added from the enforcement events page, both rewrite the
	// same policy. Adopt it into the form too, or the table keeps rendering the
	// allowlist as it stood at mount and the untouched form looks dirty against the
	// new baseline. Unsaved edits win: they are never overwritten underneath the
	// administrator.
	$effect(() => {
		const incoming = givenConfiguration;
		const hasUnsavedEdits = untrack(() => dirty);
		configuration = incoming;
		if (hasUnsavedEdits) return;
		enabled = incoming.enforcementEnabled ?? false;
		allowlist = cloneAllowlist(incoming.enforcementAllowlist);
		seededNote = false;
		// Expand the list if the incoming policy has entries, so a rule added
		// elsewhere isn't hidden behind a collapsed section.
		serversOpen =
			untrack(() => serversOpen) || (incoming.enforcementAllowlist?.servers?.length ?? 0) > 0;
		rulesOpen = untrack(() => rulesOpen) || enabled;
	});

	let servers = $derived(allowlist.servers ?? []);
	let savedState = $derived(
		JSON.stringify({
			enabled: configuration.enforcementEnabled ?? false,
			allowlist: canonicalAllowlist(configuration.enforcementAllowlist ?? {})
		})
	);
	let currentState = $derived(
		JSON.stringify({ enabled, allowlist: canonicalAllowlist(allowlist) })
	);
	let dirty = $derived(savedState !== currentState);
	let toggleChanged = $derived(enabled !== (configuration.enforcementEnabled ?? false));
	let listIsEmpty = $derived(isAllowlistEmpty(allowlist));
	let blocksEverything = $derived(enabled && listIsEmpty);
	// Only worth telling an administrator to reinstall once there is something to
	// reinstall. A configuration with no pinned bundle has no artifacts at all.
	let hasArtifacts = $derived((configuration.artifacts ?? []).length > 0);

	const tableData = $derived(
		servers.map((server, index) => ({
			id: `${index}`,
			index,
			server,
			serverDisplay: allowlistServerLabel(server),
			typeDisplay: (() => {
				const kind = allowlistServerKind(server);
				return kind
					? ALLOWLIST_SERVER_KIND_LABELS[kind]
					: m.inventory_enforcement_configuration_type_invalid();
			})(),
			toolsDisplay:
				(server.tools?.length ?? 0) === 0
					? m.inventory_enforcement_configuration_all_tools()
					: server.tools!.length === 1
						? m.inventory_enforcement_configuration_tool_count_one({ count: server.tools!.length })
						: m.inventory_enforcement_configuration_tool_count_other({
								count: server.tools!.length
							})
		}))
	);

	// Enabling enforcement on a configuration that has no policy yet would block
	// every call, so seed the sensible default the first time the toggle goes on.
	// The server only seeds on creation, and the create flow defaults to disabled,
	// so this is the path most fleets actually take. It is applied to the form, not
	// saved behind the administrator's back.
	function handleToggle(next: boolean) {
		enabled = next;
		rulesOpen = next;
		operationError = undefined;
		errorIndex = undefined;
		if (next && isAllowlistEmpty(allowlist)) {
			allowlist = defaultAllowlist();
			seededNote = true;
		} else if (!next) {
			seededNote = false;
		}
	}

	function handleServerSubmit(entry: AllowlistServer, index?: number) {
		if (index === undefined) {
			// Merge rather than append so adding a server that is already listed
			// widens or extends that entry instead of creating a second one.
			allowlist = mergeAllowlistEntry(allowlist, entry).allowlist;
			serversOpen = true;
		} else {
			const next = [...servers];
			next[index] = entry;
			allowlist = { ...allowlist, servers: next };
		}
		seededNote = false;
	}

	function removeServer(index: number) {
		allowlist = { ...allowlist, servers: servers.filter((_, i) => i !== index) };
		removingIndex = undefined;
		seededNote = false;
	}

	function reset() {
		enabled = configuration.enforcementEnabled ?? false;
		rulesOpen = enabled;
		allowlist = cloneAllowlist(configuration.enforcementAllowlist);
		operationError = undefined;
		errorIndex = undefined;
		seededNote = false;
	}

	function requestSave() {
		if (blocksEverything) {
			confirmEmpty = true;
			return;
		}
		void save();
	}

	async function save() {
		confirmEmpty = false;
		saving = true;
		operationError = undefined;
		errorIndex = undefined;
		try {
			const updated = await AdminService.updateMDMConfigurationEnforcement(configuration.id, {
				enforcementEnabled: enabled,
				enforcementAllowlist: allowlist
			});
			// Adopt the server's copy: it carries the normalized allowlist and the
			// re-rendered artifacts, and the download step upstream depends on both.
			configuration = updated;
			enabled = updated.enforcementEnabled ?? false;
			allowlist = cloneAllowlist(updated.enforcementAllowlist);
			seededNote = false;
			onUpdate(updated);
		} catch (error) {
			const problem = parseErrorContent(error);
			operationError = problem.message;
			// The server names the offending entry by index; point at that row.
			const match = problem.message.match(/entry (\d+)/);
			if (match) errorIndex = Number(match[1]);
		} finally {
			saving = false;
		}
	}
</script>

<section class="paper gap-4" id={MDM_DEVICES_CONFIGURATION_FIELD_IDS.toolCallEnforcementSection}>
	<div class="flex flex-col gap-1">
		<div class="flex flex-wrap items-center gap-2">
			<h3 class="text-lg font-semibold">
				{m.inventory_enforcement_configuration_tool_call_enforcement()}
			</h3>
			<span class="badge badge-warning badge-sm">{m.core_experimental()}</span>
		</div>
		<p class="text-muted-content text-sm font-light">
			{m.inventory_enforcement_configuration_enforcement_description()}
		</p>
	</div>

	<div class="flex items-start justify-between gap-3 text-sm">
		<button
			type="button"
			class="flex grow cursor-pointer flex-col gap-0.5 text-left"
			disabled={readOnly || saving}
			onclick={() => handleToggle(!enabled)}
		>
			<span class="font-medium">{m.inventory_enforcement_configuration_enforce_tool_calls()}</span>
			<span class="input-description">
				{m.inventory_enforcement_configuration_enforcement_toggle_description()}
			</span>
		</button>
		<div class="flex shrink-0 self-start pt-0.5">
			<Toggle
				label={enabled
					? m.inventory_enforcement_configuration_disable_enforcement()
					: m.inventory_enforcement_configuration_enable_enforcement()}
				checked={enabled}
				disabled={readOnly || saving}
				onChange={handleToggle}
			/>
		</div>
	</div>

	{#if toggleChanged && hasArtifacts}
		<div class="notification-alert flex items-start gap-2.5 p-2.5">
			<TriangleAlert class="size-4 shrink-0" />
			<span class="text-xs">
				{m.inventory_enforcement_configuration_reinstall_note()}
			</span>
		</div>
	{/if}

	{#if seededNote}
		<p class="text-muted-content text-xs">
			{m.inventory_enforcement_configuration_seeded_note()}
		</p>
	{/if}

	<div class="flex flex-col gap-3">
		<button
			type="button"
			class="flex w-fit cursor-pointer items-center gap-1.5"
			aria-expanded={rulesOpen}
			onclick={() => (rulesOpen = !rulesOpen)}
		>
			<ChevronDown class="size-4 transition-transform {rulesOpen ? '' : '-rotate-90'}" />
			<span class="input-label">{m.inventory_enforcement_configuration_allow()}</span>
		</button>

		{#if !enabled}
			<p class="text-muted-content text-xs">
				{m.inventory_enforcement_configuration_not_enforced()}
			</p>
		{/if}

		{#if rulesOpen}
			<div class="flex flex-col gap-3" in:slide={{ axis: 'y' }}>
				<label class="flex items-start gap-3 text-sm">
					<input
						type="checkbox"
						class="mt-0.5"
						checked={allowlist.allowAllObotHostedMcpServers === true}
						disabled={readOnly || saving || allowlist.allowEverything === true}
						onchange={(event) =>
							(allowlist = {
								...allowlist,
								allowAllObotHostedMcpServers: event.currentTarget.checked
							})}
					/>
					<span class="flex flex-col gap-0.5">
						<span>{m.inventory_enforcement_configuration_all_obot_hosted()}</span>
						<span class="input-description"
							>{m.inventory_enforcement_configuration_all_obot_hosted_description()}</span
						>
					</span>
				</label>

				<label class="flex items-start gap-3 text-sm">
					<input
						type="checkbox"
						class="mt-0.5"
						checked={allowlist.allowAllBuiltinAgentTools === true}
						disabled={readOnly || saving || allowlist.allowEverything === true}
						onchange={(event) =>
							(allowlist = {
								...allowlist,
								allowAllBuiltinAgentTools: event.currentTarget.checked
							})}
					/>
					<span class="flex flex-col gap-0.5">
						<span>{m.inventory_enforcement_configuration_all_builtin_tools()}</span>
						<span class="input-description">
							{m.inventory_enforcement_configuration_all_builtin_tools_description()}
						</span>
					</span>
				</label>

				<label class="flex items-start gap-3 text-sm">
					<input
						type="checkbox"
						class="mt-0.5"
						checked={allowlist.allowAllBuiltinAgentMcpServers === true}
						disabled={readOnly || saving || allowlist.allowEverything === true}
						onchange={(event) =>
							(allowlist = {
								...allowlist,
								allowAllBuiltinAgentMcpServers: event.currentTarget.checked
							})}
					/>
					<span class="flex flex-col gap-0.5">
						<span>{m.inventory_enforcement_configuration_all_builtin_mcp()}</span>
						<span class="input-description">
							{m.inventory_enforcement_configuration_all_builtin_mcp_description()}
						</span>
					</span>
				</label>

				<label class="flex items-start gap-3 text-sm">
					<input
						type="checkbox"
						class="mt-0.5"
						checked={allowlist.allowEverything === true}
						disabled={readOnly || saving}
						onchange={(event) =>
							(allowlist = { ...allowlist, allowEverything: event.currentTarget.checked })}
					/>
					<span class="flex flex-col gap-0.5">
						<span class="flex items-center gap-1.5">
							{m.inventory_enforcement_configuration_everything()}
						</span>
						<span class="input-description">
							{m.inventory_enforcement_configuration_everything_description()}
						</span>
					</span>
				</label>

				<div
					class="flex flex-col gap-3 {allowlist.allowEverything === true
						? 'pointer-events-none opacity-50'
						: ''}"
				>
					<div class="flex flex-wrap items-center justify-between gap-2">
						<button
							type="button"
							class="flex items-center gap-1.5 text-sm font-medium"
							aria-expanded={serversOpen}
							onclick={() => (serversOpen = !serversOpen)}
						>
							{m.inventory_enforcement_configuration_allowed_mcp_servers()}
							<span class="badge badge-ghost badge-sm">{servers.length}</span>
						</button>
						{#if !readOnly}
							<button
								class="btn btn-secondary btn-sm flex shrink-0 items-center gap-1"
								disabled={saving || allowlist.allowEverything === true}
								onclick={() => serverDialog?.open()}
							>
								<Plus class="size-4" />
								{m.inventory_enforcement_configuration_add()}
							</button>
						{/if}
					</div>

					{#if allowlist.allowEverything === true}
						<p class="text-muted-content text-xs">
							{m.inventory_enforcement_configuration_everything_on_note()}
						</p>
					{:else if serversOpen}
						{#if servers.length === 0}
							<div class="my-4 flex flex-col items-center gap-2 self-center text-center">
								<ShieldCheck class="text-muted-content size-12 opacity-50" />
								<p class="text-muted-content max-w-md text-sm font-light">
									{m.inventory_enforcement_configuration_no_allowed_servers()}
								</p>
							</div>
						{:else}
							<Table
								data={tableData}
								fields={['serverDisplay', 'typeDisplay', 'toolsDisplay']}
								headers={[
									{ title: m.core_col_server(), property: 'serverDisplay' },
									{ title: m.core_type(), property: 'typeDisplay' },
									{ title: m.inventory_enforcement_allowlist_tools(), property: 'toolsDisplay' }
								]}
							>
								{#snippet onRenderColumn(property, row)}
									{#if property === 'serverDisplay'}
										<span class="flex items-center gap-1.5 break-all">
											{row.serverDisplay}
											{#if errorIndex === row.index}
												<span
													use:tooltip={m.inventory_enforcement_configuration_entry_rejected()}
													role="img"
													aria-label={m.inventory_enforcement_configuration_rejected()}
												>
													<TriangleAlert class="text-error size-4 shrink-0" />
												</span>
											{/if}
										</span>
									{:else if property === 'toolsDisplay'}
										<span
											use:tooltip={row.server.tools?.length
												? row.server.tools.join(', ')
												: undefined}
										>
											{row.toolsDisplay}
										</span>
									{:else}
										{row[property as 'typeDisplay']}
									{/if}
								{/snippet}
								{#snippet actions(row)}
									{#if !readOnly}
										<DotDotDot>
											<button
												class="menu-button"
												onclick={() => serverDialog?.open(row.server, row.index)}
											>
												<Pencil class="size-4" />
												{m.inventory_enforcement_configuration_edit()}
											</button>
											<button
												class="menu-button text-error"
												onclick={() => (removingIndex = row.index)}
											>
												<Trash2 class="size-4" />
												{m.core_remove()}
											</button>
										</DotDotDot>
									{/if}
								{/snippet}
							</Table>
						{/if}
					{/if}
				</div>
			</div>
		{/if}
	</div>

	{#if blocksEverything}
		<div class="notification-alert flex items-start gap-2.5 p-2.5">
			<TriangleAlert class="size-4 shrink-0" />
			<span class="text-xs">
				{m.inventory_enforcement_configuration_blocks_everything()}
			</span>
		</div>
	{/if}

	{#if operationError}
		<p class="text-error text-xs break-all">{operationError}</p>
	{/if}

	{#if !readOnly}
		<div class="flex justify-end gap-2">
			<button class="btn btn-secondary text-sm" disabled={!dirty || saving} onclick={reset}>
				{m.core_reset_shared()}
			</button>
			<button
				class="btn btn-primary flex items-center gap-2 text-sm"
				disabled={!dirty || saving}
				onclick={requestSave}
			>
				{#if saving}<Loading class="size-4" />{:else}<Save class="size-4" />{/if}
				{m.core_save()}
			</button>
		</div>
	{/if}
</section>

<AllowlistServerDialog
	bind:this={serverDialog}
	onSubmit={(entry, index) => handleServerSubmit(entry, index)}
/>

<Confirm
	show={confirmEmpty}
	title={m.inventory_enforcement_configuration_block_every_title()}
	type="info"
	msg={m.inventory_enforcement_configuration_blocks_everything()}
	note={m.inventory_enforcement_configuration_add_rules_anytime()}
	submitText={m.inventory_enforcement_configuration_save_anyway()}
	loading={saving}
	onsuccess={save}
	oncancel={() => (confirmEmpty = false)}
/>

<Confirm
	show={removingIndex !== undefined}
	title={m.inventory_enforcement_configuration_remove_allowed_title()}
	msg={m.inventory_enforcement_configuration_remove_allowed_msg({
		server: removingIndex !== undefined ? allowlistServerLabel(servers[removingIndex]) : ''
	})}
	note={m.inventory_enforcement_configuration_takes_effect_on_save()}
	submitText={m.core_remove()}
	onsuccess={() => removingIndex !== undefined && removeServer(removingIndex)}
	oncancel={() => (removingIndex = undefined)}
/>
