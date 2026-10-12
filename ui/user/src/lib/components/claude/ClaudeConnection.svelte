<script lang="ts">
	import CopyField from '$lib/components/CopyField.svelte';
	import Select from '$lib/components/Select.svelte';
	import { m } from '$lib/i18n';
	import { Group, UserService, type VMCP } from '$lib/services';
	import { doGet, doPost, doPut, doDelete } from '$lib/services/http';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import ActorResources from './ActorResources.svelte';
	import ClaudeAccountMappings from './ClaudeAccountMappings.svelte';
	import ClaudeTagSettings from './ClaudeTagSettings.svelte';
	import EgressHosts from './EgressHosts.svelte';
	import EgressSecrets from './EgressSecrets.svelte';
	import EnvironmentProfiles from './EnvironmentProfiles.svelte';
	import type { ClaudeConfig, ClaudeEnvironment } from './types';
	import { onMount } from 'svelte';

	let config = $state<ClaudeConfig>({ enabled: false });
	let servers = $state<VMCP[]>([]);
	let sessions = $state<
		{
			metadata: { deletionTimestamp?: string };
			spec: { claudeEnvironment: string; claudeDone?: boolean; claudeFinishedAt?: string };
			status: { state?: string };
		}[]
	>([]);
	let statsLoaded = $state(false);
	let statsError = $state(false);
	let refreshingStats = false;

	const stats = $derived.by(() => {
		const result: Record<string, { total: number; active: number; lastUsed?: string }> = {};

		for (const session of sessions) {
			const entry = (result[session.spec.claudeEnvironment] ??= { total: 0, active: 0 });
			entry.total++;

			if (
				session.status.state === 'RUNNING' &&
				!session.spec.claudeDone &&
				!session.metadata.deletionTimestamp
			)
				entry.active++;

			const finished = session.spec.claudeFinishedAt;
			if (finished && Date.parse(finished) > Date.parse(entry.lastUsed ?? '1970-01-01'))
				entry.lastUsed = finished;
		}

		return result;
	});

	async function refreshStats() {
		if (refreshingStats || !config.enabled) return;
		refreshingStats = true;

		try {
			const response = (await doGet(admin ? '/claude-sessions?all=true' : '/claude-sessions', {
				dontLogErrors: true
			})) as { items: typeof sessions };
			sessions = response.items;
			statsLoaded = true;
			statsError = false;
		} catch {
			statsError = true;
		} finally {
			refreshingStats = false;
		}
	}
	let draft = $state<ClaudeEnvironment & { environmentKey: string }>();
	let managedSettings = $state('');
	let nodeSelector = $state('kubernetes.io/os=linux');
	let error = $state('');
	let busy = $state(false);
	const admin = $derived(profile.current.isAdmin?.() ?? false);
	async function load() {
		config = (await doGet('/claude-environments')) as ClaudeConfig;
		void refreshStats();
		servers = (await UserService.listVMCPs(admin ? { all: true } : {})).filter(
			(server) => !admin || !server.userID
		);
	}
	onMount(() => {
		void load().catch((e) => (error = String(e)));
		const timer = setInterval(() => void refreshStats(), 5000);
		return () => clearInterval(timer);
	});
	function create() {
		draft = {
			id: '',
			displayName: '',
			organizationID: '',
			environmentID: '',
			environmentKey: '',
			mcpServerIDs: [],
			claudeTagEnabled: false,
			profiles: [
				{
					name: 'default',
					subjects: [{ type: 'obotGroup', id: Group.ADMIN }],
					mcpServerIDs: null,
					resources: {}
				}
			],
			runtime: {
				image: config.defaultRunnerImage ?? '',
				workerImage: config.defaultWorkerImage ?? '',
				allowAllEgress: false,
				egressHosts: [],
				worker: {
					workerReplicas: 1,
					cpuLimit: '2',
					memoryLimit: '4Gi'
				},
				resources: {
					actorCPU: '1',
					actorMemory: '2Gi',
					idleTimeoutMinutes: 5,
					sessionRetentionDays: 30
				}
			}
		};
		managedSettings = '';
		nodeSelector = 'kubernetes.io/os=linux';
	}
	function edit(environment: ClaudeEnvironment) {
		draft = { ...structuredClone($state.snapshot(environment)), environmentKey: '' };
		managedSettings = draft.runtime.managedSettings
			? JSON.stringify(draft.runtime.managedSettings, null, 2)
			: '';
		nodeSelector = Object.entries(draft.runtime.worker.nodeSelector ?? {})
			.map(([key, value]) => `${key}=${value}`)
			.join('\n');
	}
	async function save(event: SubmitEvent) {
		event.preventDefault();
		if (!draft) return;
		busy = true;
		error = '';
		draft.runtime.egressHosts = draft.runtime.egressHosts
			.map((host) => host.trim())
			.filter(Boolean);
		for (const accessProfile of draft.profiles) {
			accessProfile.egressHosts = accessProfile.egressHosts
				?.map((host) => host.trim())
				.filter(Boolean);
			if (accessProfile.mcpServerIDs !== null)
				accessProfile.mcpServerIDs = accessProfile.mcpServerIDs.filter((id) =>
					draft?.mcpServerIDs.includes(id)
				);
		}
		try {
			const settings = managedSettings.trim() ? JSON.parse(managedSettings) : undefined;
			if (
				settings !== undefined &&
				(settings === null || typeof settings !== 'object' || Array.isArray(settings))
			) {
				throw new Error(m.hosted_agents_managed_settings_object());
			}
			draft.runtime.managedSettings = settings;

			const selectors: Record<string, string> = {};
			for (const line of nodeSelector
				.split('\n')
				.map((line) => line.trim())
				.filter(Boolean)) {
				const equals = line.indexOf('=');
				if (equals < 1) throw new Error(m.hosted_agents_env_selector_error());
				selectors[line.slice(0, equals)] = line.slice(equals + 1);
			}
			draft.runtime.worker.nodeSelector = selectors;
			const payload = { ...draft, organizationID: undefined, environmentID: undefined };
			if (draft.id) await doPut(`/claude-environments/${draft.id}`, payload);
			else await doPost('/claude-environments', payload);
			draft = undefined;
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}
	async function remove(environment: ClaudeEnvironment) {
		if (!confirm(m.hosted_agents_env_delete_question({ name: environment.displayName }))) return;
		busy = true;
		error = '';
		try {
			await doDelete(`/claude-environments/${environment.id}`);
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}
</script>

{#if error}<p role="alert" class="text-error">{error}</p>{/if}
{#if config.enabled}
	<section class="space-y-4" aria-label={m.hosted_agents_env_heading()}>
		<p class="text-sm text-muted-content">
			{m.hosted_agents_env_help()}
		</p>
		{#if admin}<button class="btn btn-primary btn-sm" disabled={busy} onclick={create}
				>{m.hosted_agents_env_add()}</button
			>{/if}
		{#if admin && draft}
			<form
				class="space-y-5 rounded-xl border border-base-300 bg-base-100 p-5 shadow-sm"
				onsubmit={save}
			>
				<h2 class="font-semibold">
					{draft.id ? m.hosted_agents_env_edit() : m.hosted_agents_env_add()}
				</h2>
				<fieldset disabled={busy} class="space-y-4">
					<label class="flex flex-col gap-1 text-sm"
						>{m.hosted_agents_env_name()}<input
							class="text-input-filled"
							required
							maxlength="200"
							bind:value={draft.displayName}
						/></label
					>
					<label class="flex flex-col gap-1 text-sm"
						>{m.hosted_agents_env_key()}<input
							class="text-input-filled"
							type="password"
							autocomplete="new-password"
							required={!draft.id}
							bind:value={draft.environmentKey}
							placeholder={draft.id ? m.hosted_agents_env_keep_key() : ''}
						/></label
					>
					<fieldset class="space-y-2">
						<legend class="font-semibold">{m.hosted_agents_available_vmcps()}</legend>
						<p class="text-xs text-muted-content">
							{m.hosted_agents_env_vmcp_help()}
						</p>
						<Select
							id="environment-vmcps"
							options={servers
								.toSorted(
									(a, b) =>
										Number(draft?.mcpServerIDs?.includes(b.id)) -
										Number(draft?.mcpServerIDs?.includes(a.id))
								)
								.map((server) => ({
									id: server.id,
									label: server.displayName || server.id
								}))}
							bind:selected={
								() => draft?.mcpServerIDs.join(',') ?? '',
								(value) => {
									if (draft)
										draft.mcpServerIDs = String(value ?? '')
											.split(',')
											.filter(Boolean);
								}
							}
							multiple
							searchInDropdown
							buttonReadOnly
							placeholder={m.hosted_agents_select_vmcps()}
							searchPlaceholder={m.hosted_agents_search_vmcps()}
							classes={{ root: 'w-full' }}
						/>
					</fieldset>
					<fieldset class="space-y-3">
						<legend class="font-semibold">{m.hosted_agents_env_pool()}</legend>

						<div class="grid gap-3 sm:grid-cols-2">
							<label class="flex flex-col gap-1 text-sm"
								>{m.hosted_agents_env_replicas()}<input
									class="text-input-filled"
									type="number"
									min="1"
									max="100"
									required
									bind:value={draft.runtime.worker.workerReplicas}
								/></label
							>
							<label class="flex flex-col gap-1 text-sm"
								>{m.hosted_agents_env_cpu()}<input
									class="text-input-filled"
									required
									bind:value={draft.runtime.worker.cpuLimit}
								/></label
							>
							<label class="flex flex-col gap-1 text-sm"
								>{m.hosted_agents_env_memory()}<input
									class="text-input-filled"
									required
									bind:value={draft.runtime.worker.memoryLimit}
								/></label
							>
						</div>
						<p class="text-xs text-muted-content">
							{m.hosted_agents_env_capacity_help()}
						</p>
						<details class="space-y-3">
							<summary class="cursor-pointer text-sm font-medium"
								>{m.hosted_agents_env_advanced()}</summary
							><label class="flex flex-col gap-1 text-sm"
								>{m.hosted_agents_env_selectors()}<textarea
									class="text-input-filled"
									bind:value={nodeSelector}
									placeholder={m.hosted_agents_env_selector_placeholder()}></textarea></label
							>
							<label class="flex flex-col gap-1 text-sm"
								>{m.hosted_agents_env_worker_image()}<input
									class="text-input-filled"
									required
									bind:value={draft.runtime.workerImage}
								/></label
							>
						</details>
					</fieldset>
					<fieldset class="space-y-3">
						<legend class="font-semibold">{m.hosted_agents_env_actor_defaults()}</legend>

						<ActorResources bind:resources={draft.runtime.resources} />
						<p class="text-xs text-muted-content">
							{m.hosted_agents_env_digest_help()}
						</p>
						<details class="space-y-3">
							<summary class="cursor-pointer text-sm font-medium"
								>{m.hosted_agents_env_advanced()}</summary
							><label class="flex flex-col gap-1 text-sm"
								>{m.hosted_agents_env_runner_image()}<input
									class="text-input-filled"
									required
									bind:value={draft.runtime.image}
									placeholder="registry/obot-claude@sha256:…"
									pattern={'[^@\\s]+@sha256:[a-f0-9]{64}'}
									maxlength={512}
									title={m.hosted_agents_env_digest_title()}
									oninvalid={(event) => {
										const details = event.currentTarget.closest('details');
										if (details) details.open = true;
									}}
								/></label
							>
							<label class="flex flex-col gap-1 text-sm">
								{m.hosted_agents_managed_settings_label()}
								<textarea
									class="text-input-filled font-mono"
									rows="10"
									bind:value={managedSettings}
									placeholder={'{}'}></textarea>
							</label>
							<p class="text-xs text-muted-content">{m.hosted_agents_managed_settings_help()}</p>
							<label class="flex items-center gap-2 text-sm">
								<input type="checkbox" bind:checked={draft.runtime.allowAllEgress} />
								{m.hosted_agents_allow_all_egress()}
							</label>
							<p class="text-xs text-muted-content">{m.hosted_agents_allow_all_egress_help()}</p>
							<p class="text-xs text-muted-content">{m.hosted_agents_required_egress_help()}</p>
							<EgressHosts
								label={m.hosted_agents_environment_egress_hosts()}
								bind:hosts={draft.runtime.egressHosts}
								disabled={draft.runtime.allowAllEgress}
							/>
						</details>
					</fieldset>
					<EgressSecrets
						bind:secrets={
							() => draft?.egressSecrets ?? [],
							(secrets) => {
								if (draft) draft.egressSecrets = secrets;
							}
						}
					/>
					<EnvironmentProfiles
						bind:profiles={draft.profiles}
						servers={servers.filter((server) => draft?.mcpServerIDs.includes(server.id))}
						disabled={busy}
					/>
					<p class="text-xs text-muted-content">
						{m.hosted_agents_env_changes_help()}
					</p>
				</fieldset>
				<div class="flex justify-end gap-2">
					<button
						class="btn btn-ghost"
						type="button"
						disabled={busy}
						onclick={() => (draft = undefined)}>{m.common_cancel()}</button
					><button class="btn btn-primary" type="submit" disabled={busy}
						>{m.hosted_agents_env_save()}</button
					>
				</div>
			</form>
		{/if}
		{#if admin}
			{#each [...new Set((config.environments ?? []).map((environment) => environment.organizationID))] as organization (organization)}
				<ClaudeAccountMappings
					{organization}
					environments={(config.environments ?? []).filter(
						(environment) => environment.organizationID === organization
					)}
					onsaved={load}
				/>
			{/each}
		{/if}
		{#each config.environments ?? [] as environment (environment.id)}
			{@const mapping = config.mappings?.find(
				(mapping) => mapping.organizationID === environment.organizationID
			)}
			<article class="space-y-3 rounded-xl border border-base-300 bg-base-100 p-5 shadow-sm">
				<div class="flex items-center justify-between gap-2">
					<h2 class="font-semibold">{environment.displayName}</h2>
					{#if admin}<div class="flex gap-2">
							<button class="btn btn-ghost btn-sm" disabled={busy} onclick={() => edit(environment)}
								>{m.hosted_agents_env_action_edit()}</button
							><button
								class="btn btn-ghost btn-sm text-error"
								disabled={busy}
								onclick={() => remove(environment)}>{m.hosted_agents_env_action_delete()}</button
							>
						</div>{/if}
				</div>
				<p class="text-sm">
					{m.hosted_agents_env_id_label()} <code>{environment.environmentID}</code>
				</p>
				<div class="space-y-1">
					<CopyField
						id={`claude-cli-${environment.id}`}
						label={m.hosted_agents_cli_command()}
						variant="code"
						value={`claude "${m.hosted_agents_cli_prompt()}" --environment ${environment.environmentID}`}
					/>
					<p class="text-xs text-muted-content">{m.hosted_agents_cli_help()}</p>
				</div>
				{#if statsError}
					<p role="alert" class="text-error text-sm">{m.hosted_agents_environment_stats_error()}</p>
				{:else if !statsLoaded}
					<p role="status" class="text-sm text-muted-content">
						{m.hosted_agents_environment_stats_loading()}
					</p>
				{:else}
					{@const summary = stats[environment.id] ?? { total: 0, active: 0 }}
					{@const lastUsed = formatTimeAgo(summary.lastUsed)}
					<p class="text-xs text-muted-content">
						{admin
							? m.hosted_agents_environment_stats_all()
							: m.hosted_agents_environment_stats_own()}
					</p>
					<dl class="grid grid-cols-3 gap-3 text-sm">
						<div>
							<dt class="text-muted-content">{m.hosted_agents_environment_stats_total()}</dt>
							<dd>{summary.total}</dd>
						</div>
						<div>
							<dt class="text-muted-content">{m.hosted_agents_environment_stats_active()}</dt>
							<dd>{summary.active}</dd>
						</div>
						<div>
							<dt class="text-muted-content">{m.hosted_agents_environment_stats_last_used()}</dt>
							<dd>
								{#if summary.active}
									{m.hosted_agents_environment_stats_now()}
								{:else if lastUsed.relativeTime}
									<time datetime={summary.lastUsed} title={lastUsed.fullDate}
										>{lastUsed.relativeTime}</time
									>
								{:else}
									{summary.total
										? m.hosted_agents_environment_stats_unknown()
										: m.hosted_agents_environment_stats_never()}
								{/if}
							</dd>
						</div>
					</dl>
				{/if}
				{#if admin}
					<p class="text-sm">
						{m.hosted_agents_env_summary({
							count: environment.mcpServerIDs?.length ?? 0,
							cpu: environment.runtime.resources.actorCPU ?? '',
							memory: environment.runtime.resources.actorMemory ?? ''
						})}
					</p>
				{:else}
					<section class="space-y-3" aria-label={m.hosted_agents_configuration()}>
						<p class="text-xs text-muted-content">{m.hosted_agents_configuration_help()}</p>
						<div>
							<h3 class="text-sm font-semibold">{m.hosted_agents_available_vmcps()}</h3>
							<ul class="mt-1 space-y-1 text-sm">
								{#each environment.mcpServerIDs ?? [] as id (id)}
									<li>{servers.find((server) => server.id === id)?.displayName || id}</li>
								{:else}<li class="text-muted-content">{m.hosted_agents_no_vmcps()}</li>{/each}
							</ul>
						</div>
						<div>
							<h3 class="text-sm font-semibold">{m.hosted_agents_allowed_egress()}</h3>
							<p class="mt-1 text-xs text-muted-content">
								{m.hosted_agents_required_egress_help()}
							</p>
							{#if environment.runtime.allowAllEgress}
								<p class="text-sm">{m.hosted_agents_all_egress_allowed()}</p>
							{:else}
								<ul class="mt-1 space-y-1 text-sm">
									{#each environment.runtime.egressHosts ?? [] as host (host)}
										<li><code>{host}</code></li>
									{:else}<li class="text-muted-content">
											{m.hosted_agents_no_additional_egress()}
										</li>{/each}
								</ul>
							{/if}
						</div>
					</section>
				{/if}
				{#if mapping}<p class="text-sm">
						{m.hosted_agents_env_account_label()} <code>{mapping.account}</code> ({mapping.email})
					</p>{:else}<p class="text-sm text-muted-content">
						{m.hosted_agents_env_ask_admin()}
					</p>{/if}
				{#if admin}<ClaudeTagSettings {environment} onsaved={load} />{/if}
			</article>
		{:else}<p class="text-muted-content">{m.hosted_agents_env_empty()}</p>{/each}
	</section>
{:else}<p class="text-muted-content">{m.hosted_agents_env_disabled()}</p>{/if}
