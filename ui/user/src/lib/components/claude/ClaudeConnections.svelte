<script lang="ts">
	import CopyField from '$lib/components/CopyField.svelte';
	import { m } from '$lib/i18n';
	import {
		UserService,
		type ClaudeSession,
		type ClaudeConfig,
		type ClaudeEnvironment
	} from '$lib/services';
	import { COMMON_AI_CLIENTS_MAP, AiClient } from '$lib/services/user/constants';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { Pencil } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	let { onEdit }: { onEdit: (environment: ClaudeEnvironment) => void } = $props();

	let config = $state<ClaudeConfig>({ enabled: false });
	let sessions = $state<ClaudeSession[]>([]);
	let statsLoaded = $state(false);
	let statsError = $state(false);
	let refreshingStats = false;
	let error = $state('');
	const admin = $derived(profile.current.isAdmin?.() ?? false);

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
			sessions = await UserService.listClaudeSessions({
				all: admin,
				dontLogErrors: true
			});
			statsLoaded = true;
			statsError = false;
		} catch {
			statsError = true;
		} finally {
			refreshingStats = false;
		}
	}

	async function load() {
		config = await UserService.getClaudeConfig();
		void refreshStats();
	}

	onMount(() => {
		void load().catch((e) => (error = String(e)));
		const timer = setInterval(() => void refreshStats(), 5000);
		return () => clearInterval(timer);
	});
</script>

{#if error}<p role="alert" class="text-error">{error}</p>{/if}
{#if config.enabled}
	<section class="space-y-4" aria-label={m.hosted_agents_env_heading()}>
		<p class="text-sm text-muted-content">
			{m.hosted_agents_env_help()}
		</p>
		<div class="space-y-4">
			{#each config.environments ?? [] as environment (environment.id)}
				{@const mapping = config.mappings?.find(
					(mapping) => mapping.organizationID === environment.organizationID
				)}
				<article
					class={twMerge(
						'relative group space-y-3 h-full text-base-content border-base-300 dark:border-base-300 bg-base-100 dark:bg-base-200 group @container gap-3 rounded-lg border p-4 shadow-xs',
						admin &&
							'pointer-events-none cursor-pointer transition-[transform,box-shadow,border-color] duration-150 hover:border-primary/50 hover:shadow-md dark:hover:border-primary/50'
					)}
				>
					{#if admin}
						<button
							type="button"
							class="pointer-events-auto absolute inset-0 z-0 cursor-pointer rounded-[inherit] focus-visible:ring-2 focus-visible:ring-primary focus-visible:outline-none"
							aria-label={m.hosted_agents_env_edit_named({ name: environment.displayName })}
							onclick={() => onEdit(environment)}
						></button>
						<div class="absolute top-4 right-4 group-hover:block hidden">
							<Pencil class="size-5 text-primary" />
						</div>
					{/if}
					<h2 class="font-semibold flex items-center gap-1 flex-wrap">
						{environment.displayName}
						<span class="text-xs text-muted-content font-normal">
							({m.hosted_agents_env_id_label()} <code>{environment.environmentID}</code>)
						</span>
					</h2>

					{#if admin}
						<p class="text-xs text-muted-content">
							{m.hosted_agents_env_summary({
								cpu: environment.runtime.resources.actorCPU ?? '',
								memory: environment.runtime.resources.actorMemory ?? ''
							})}
						</p>
					{:else}
						<section class="space-y-3" aria-label={m.hosted_agents_configuration()}>
							<div>
								<h3 class="text-xs font-normal mb-1">{m.hosted_agents_allowed_egress()}</h3>
								{#if environment.runtime.allowAllEgress}
									<p class="text-sm">{m.hosted_agents_all_egress_allowed()}</p>
								{:else}
									<ul class="flex flex-wrap items-center gap-2">
										{#each environment.runtime.egressHosts ?? [] as host (host)}
											<li
												class="badge badge-sm badge-outline badge-primary"
												title={m.hosted_agents_egress_host_allowed({ host })}
											>
												<code>{host}</code>
											</li>
										{:else}
											<li class="text-muted-content">
												{m.hosted_agents_no_additional_egress()}
											</li>
										{/each}
									</ul>
								{/if}
							</div>
						</section>
					{/if}

					<div class="divider"></div>
					<div class="space-y-2">
						<div class="pointer-events-auto relative z-10 space-y-0.5">
							<CopyField
								id={`claude-cli-${environment.id}`}
								label={m.hosted_agents_cli_command()}
								variant="code"
								value={`claude "${m.hosted_agents_cli_prompt()}" --environment ${environment.environmentID}`}
								classes={{
									input: 'font-mono dark:bg-base-300 pt-0.5 pl-4',
									label: 'text-sm text-base-content mb-1'
								}}
								class="dark:bg-base-300"
							>
								{#snippet preContent()}
									{@const claude = COMMON_AI_CLIENTS_MAP.get(AiClient.Claude)}
									{#if claude}
										<span class="label shrink-0 w-20 mr-0 text-base-content">
											<img
												src={claude.iconDark ?? claude.icon}
												alt={m.mcps_connect_branding_icon_alt({ name: claude.alt ?? '' })}
												class="size-4 dark:block hidden"
											/>
											<img
												src={claude.icon}
												alt={m.mcps_connect_branding_icon_alt({ name: claude.alt ?? '' })}
												class="size-4 block dark:hidden"
											/>
											{claude.alt}
										</span>
									{/if}
								{/snippet}
							</CopyField>
						</div>
						<p class="text-xs font-light">{m.hosted_agents_cli_help()}</p>
					</div>
					<div class="divider"></div>
					{#if statsError}
						<p role="alert" class="text-error text-sm">
							{m.hosted_agents_environment_stats_error()}
						</p>
					{:else if !statsLoaded}
						<p role="status" class="text-sm text-muted-content">
							{m.hosted_agents_environment_stats_loading()}
						</p>
					{:else}
						{@const summary = stats[environment.id] ?? { total: 0, active: 0 }}
						{@const lastUsed = formatTimeAgo(summary.lastUsed)}
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
					<div class="flex justify-between items-center gap-4 flex-wrap mt-4">
						{#if !statsError && statsLoaded}
							<p class="text-xs text-muted-content font-light">
								{admin
									? m.hosted_agents_environment_stats_all()
									: m.hosted_agents_environment_stats_own()}
							</p>
						{/if}
						<p class="text-xs text-muted-content font-light">
							{#if mapping}
								{m.hosted_agents_env_account_label()} <code>{mapping.account}</code>
								({mapping.email})
							{:else}
								{m.hosted_agents_env_ask_admin()}
							{/if}
						</p>
					</div>
				</article>
			{:else}
				<p class="text-muted-content">{m.hosted_agents_env_empty()}</p>
			{/each}
		</div>
	</section>
{:else}<p class="text-muted-content">{m.hosted_agents_env_disabled()}</p>{/if}
