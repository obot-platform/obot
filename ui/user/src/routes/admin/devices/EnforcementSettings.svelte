<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Table from '$lib/components/table/Table.svelte';
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
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type AllowlistServer,
		type EnforcementAllowlist,
		type MDMConfiguration
	} from '$lib/services';
	import AllowlistServerDialog from './AllowlistServerDialog.svelte';
	import { Pencil, Plus, Save, ShieldCheck, Trash2, TriangleAlert } from '@lucide/svelte';
	import { untrack } from 'svelte';

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

	$effect(() => {
		configuration = givenConfiguration;
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
				return kind ? ALLOWLIST_SERVER_KIND_LABELS[kind] : 'Invalid';
			})(),
			toolsDisplay:
				(server.tools?.length ?? 0) === 0
					? 'All tools'
					: `${server.tools!.length} ${server.tools!.length === 1 ? 'tool' : 'tools'}`
		}))
	);

	// Enabling enforcement on a configuration that has no policy yet would block
	// every call, so seed the sensible default the first time the toggle goes on.
	// The server only seeds on creation, and the create flow defaults to disabled,
	// so this is the path most fleets actually take. It is applied to the form, not
	// saved behind the administrator's back.
	function handleToggle(next: boolean) {
		enabled = next;
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

<section class="paper gap-4">
	<div class="flex flex-col gap-1">
		<h3 class="text-lg font-semibold">Tool Call Enforcement</h3>
		<p class="text-muted-content text-sm font-light">
			Control exactly which tool calls enrolled devices may execute. Calls that aren't allowed are
			blocked on the device.
		</p>
	</div>

	<label class="flex items-start gap-3 text-sm">
		<input
			type="checkbox"
			class="mt-0.5"
			checked={enabled}
			disabled={readOnly || saving}
			onchange={(event) => handleToggle(event.currentTarget.checked)}
		/>
		<span class="flex flex-col gap-0.5">
			<span class="flex flex-wrap items-center gap-1.5 font-medium">
				Enforce tool calls on enrolled devices
				<span class="badge badge-warning badge-sm">Experimental</span>
			</span>
			<span class="input-description">
				Installs enforcement hooks alongside Obot Sentry. Every tool call is checked against the
				rules below before it runs. This feature is experimental and is not recommended for
				production use — a misconfigured allowlist blocks real work on every enrolled device.
			</span>
		</span>
	</label>

	{#if toggleChanged && hasArtifacts}
		<div class="notification-alert flex items-start gap-2.5 p-2.5">
			<TriangleAlert class="size-4 shrink-0" />
			<span class="text-xs">
				Enforcement hooks are installed by the Obot Sentry package. Re-download the install package
				above and reinstall it on your devices for this change to take effect.
			</span>
		</div>
	{/if}

	{#if seededNote}
		<p class="text-muted-content text-xs">
			Started you off with a default policy. Review it and save, or adjust it first.
		</p>
	{/if}

	<div class="flex flex-col gap-3">
		<span class="input-label">Allow</span>

		{#if !enabled}
			<p class="text-muted-content text-xs">
				Not currently enforced — these rules take effect when enforcement is enabled.
			</p>
		{/if}

		<label class="flex items-start gap-3 text-sm">
			<input
				type="checkbox"
				class="mt-0.5"
				checked={allowlist.allowAllObotHostedMcpServers === true}
				disabled={readOnly || saving || allowlist.allowEverything === true}
				onchange={(event) =>
					(allowlist = { ...allowlist, allowAllObotHostedMcpServers: event.currentTarget.checked })}
			/>
			<span class="flex flex-col gap-0.5">
				<span>All Obot-hosted MCP servers</span>
				<span class="input-description">Any MCP server hosted by this Obot instance.</span>
			</span>
		</label>

		<label class="flex items-start gap-3 text-sm">
			<input
				type="checkbox"
				class="mt-0.5"
				checked={allowlist.allowAllBuiltinAgentTools === true}
				disabled={readOnly || saving || allowlist.allowEverything === true}
				onchange={(event) =>
					(allowlist = { ...allowlist, allowAllBuiltinAgentTools: event.currentTarget.checked })}
			/>
			<span class="flex flex-col gap-0.5">
				<span>All built-in agent tools</span>
				<span class="input-description">
					The agent's own tools, like reading files, writing files, and running shell commands.
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
				<span>All built-in agent MCP servers</span>
				<span class="input-description">
					MCP servers that ship inside the coding agent itself.
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
					Everything
					<TriangleAlert class="text-warning size-3.5" />
				</span>
				<span class="input-description"> Allows every tool call and ignores all other rules. </span>
			</span>
		</label>
	</div>

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
				Allowed MCP servers
				<span class="badge badge-ghost badge-sm">{servers.length}</span>
			</button>
			{#if !readOnly}
				<button
					class="btn btn-secondary btn-sm flex shrink-0 items-center gap-1"
					disabled={saving || allowlist.allowEverything === true}
					onclick={() => serverDialog?.open()}
				>
					<Plus class="size-4" />
					Add
				</button>
			{/if}
		</div>

		{#if allowlist.allowEverything === true}
			<p class="text-muted-content text-xs">
				All other rules are ignored while "Everything" is on.
			</p>
		{:else if serversOpen}
			{#if servers.length === 0}
				<div class="my-4 flex flex-col items-center gap-2 self-center text-center">
					<ShieldCheck class="text-muted-content size-12 opacity-50" />
					<p class="text-muted-content max-w-md text-sm font-light">
						No specific MCP servers allowed. Add one to allow calls to a server that isn't covered
						by the rules above.
					</p>
				</div>
			{:else}
				<Table
					data={tableData}
					fields={['serverDisplay', 'typeDisplay', 'toolsDisplay']}
					headers={[
						{ title: 'Server', property: 'serverDisplay' },
						{ title: 'Type', property: 'typeDisplay' },
						{ title: 'Tools', property: 'toolsDisplay' }
					]}
				>
					{#snippet onRenderColumn(property, row)}
						{#if property === 'serverDisplay'}
							<span class="flex items-center gap-1.5 break-all">
								{row.serverDisplay}
								{#if errorIndex === row.index}
									<span use:tooltip={'This entry was rejected'} role="img" aria-label="Rejected">
										<TriangleAlert class="text-error size-4 shrink-0" />
									</span>
								{/if}
							</span>
						{:else if property === 'toolsDisplay'}
							<span
								use:tooltip={row.server.tools?.length ? row.server.tools.join(', ') : undefined}
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
									Edit
								</button>
								<button class="menu-button text-error" onclick={() => (removingIndex = row.index)}>
									<Trash2 class="size-4" />
									Remove
								</button>
							</DotDotDot>
						{/if}
					{/snippet}
				</Table>
			{/if}
		{/if}
	</div>

	{#if blocksEverything}
		<div class="notification-alert flex items-start gap-2.5 p-2.5">
			<TriangleAlert class="size-4 shrink-0" />
			<span class="text-xs">
				Enforcement is enabled but nothing is allowed. Every tool call on every enrolled device will
				be blocked.
			</span>
		</div>
	{/if}

	{#if operationError}
		<p class="text-error text-xs break-all">{operationError}</p>
	{/if}

	{#if !readOnly}
		<div class="flex justify-end gap-2">
			<button class="btn btn-secondary text-sm" disabled={!dirty || saving} onclick={reset}>
				Reset
			</button>
			<button
				class="btn btn-primary flex items-center gap-2 text-sm"
				disabled={!dirty || saving}
				onclick={requestSave}
			>
				{#if saving}<Loading class="size-4" />{:else}<Save class="size-4" />{/if}
				Save
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
	title="Block every tool call?"
	type="info"
	msg="Enforcement is enabled but nothing is allowed. Every tool call on every enrolled device will be blocked."
	note="You can add rules at any time."
	submitText="Save anyway"
	loading={saving}
	onsuccess={save}
	oncancel={() => (confirmEmpty = false)}
/>

<Confirm
	show={removingIndex !== undefined}
	title="Remove allowed server"
	msg={`Remove "${removingIndex !== undefined ? allowlistServerLabel(servers[removingIndex]) : ''}" from the allowlist? Calls to it will be blocked unless another rule allows them.`}
	note="This takes effect when you save."
	submitText="Remove"
	onsuccess={() => removingIndex !== undefined && removeServer(removingIndex)}
	oncancel={() => (removingIndex = undefined)}
/>
