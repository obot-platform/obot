<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import { m } from '$lib/i18n';
	import { UserService, type OrgUser } from '$lib/services';
	import { doDelete, doGet, doPost } from '$lib/services/http';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { Bot, ExternalLink, Pause, Trash2 } from '@lucide/svelte';
	import { onMount } from 'svelte';

	let { all = false }: { all?: boolean } = $props();

	type Session = {
		metadata: { name: string; deletionTimestamp?: string };
		spec: {
			displayName: string;
			claudeSession: string;
			userID: string;
			claudeEnvironment: string;
			claudeAccount: string;
			suspended?: boolean;
			claudeDone?: boolean;
			claudeFinishedAt?: string;
			slackSenderName?: string;
			slackSenderHandle?: string;
			slackChannelName?: string;
			slackLink?: string;
			claudeRepositories?: { url: string; revision?: string }[] | null;
		};
		status: { state?: string; error?: string };
	};
	let sessions = $state<Session[]>([]);
	let users = $state<Record<string, OrgUser>>({});
	let userError = $state('');
	let loading = $state(true);
	let error = $state('');
	let deleting = $state('');
	let suspending = $state('');
	let pendingSuspend = $state<Session>();
	let refreshing = false;
	let pendingDelete = $state<Session>();

	async function refresh() {
		if (refreshing) return;
		refreshing = true;
		try {
			sessions = (
				(await doGet(all ? '/claude-sessions?all=true' : '/claude-sessions', {
					dontLogErrors: true
				})) as { items: Session[] }
			).items;
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
			refreshing = false;
		}
	}

	onMount(() => {
		void refresh();
		if (all) {
			void UserService.listUsersIncludeDeleted()
				.then((directory) => {
					users = Object.fromEntries(directory.map((user) => [user.id, user]));
				})
				.catch(() => (userError = m.hosted_agents_sessions_user_error()));
		}
		const timer = setInterval(() => void refresh(), 5000);
		return () => clearInterval(timer);
	});

	async function suspend(session: Session) {
		suspending = session.metadata.name;
		try {
			await doPost(`/claude-sessions/${encodeURIComponent(session.metadata.name)}/suspend`, {});
			pendingSuspend = undefined;
			await refresh();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			suspending = '';
		}
	}

	async function remove(session: Session) {
		deleting = session.metadata.name;
		try {
			await doDelete(`/claude-sessions/${encodeURIComponent(session.metadata.name)}`);
			pendingDelete = undefined;
			await refresh();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			deleting = '';
		}
	}

	function sessionState(state?: string) {
		switch (state?.toUpperCase()) {
			case 'RUNNING':
				return m.hosted_agents_sessions_running();
			case 'SUSPENDED':
				return m.hosted_agents_sessions_suspended();
			case 'RESUMING':
				return m.hosted_agents_sessions_resuming();
			case 'ERROR':
				return m.hosted_agents_sessions_error();
			case 'PENDING':
			case undefined:
			case '':
				return m.hosted_agents_sessions_pending();
			default:
				return state;
		}
	}
</script>

<div class="flex w-full flex-col gap-4">
	<p class="text-sm font-light text-muted-content">
		{#if all}
			{m.hosted_agents_sessions_all_help()}
		{:else}
			{m.hosted_agents_sessions_help()}
		{/if}
	</p>
	{#if error}<p role="alert" class="text-error">{error}</p>{/if}
	{#if userError}<p role="alert" class="text-error">{userError}</p>{/if}
	{#if loading}
		<p class="badge badge-soft badge-sm" role="status">{m.hosted_agents_sessions_loading()}</p>
	{:else}
		<ul class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
			{#each sessions as session (session.metadata.name)}
				{@const user = users[session.spec.userID]}
				{@const lastUsed = formatTimeAgo(session.spec.claudeFinishedAt ?? undefined)}
				<li
					class="flex min-w-0 flex-col gap-3 rounded-xl border border-base-300 bg-base-100 p-4 shadow-sm"
				>
					<h2 class="flex items-center gap-2 text-sm font-semibold">
						<Bot class="size-5 shrink-0 text-primary" />{session.spec.displayName}
					</h2>
					<p class="badge badge-soft badge-sm" role="status">
						{session.metadata.deletionTimestamp
							? m.hosted_agents_sessions_deleting()
							: session.spec.suspended && !session.spec.claudeDone
								? m.hosted_agents_sessions_suspending()
								: sessionState(session.status.state)}
					</p>
					{#if all}
						<dl class="text-sm break-all">
							<dt class="text-muted-content">{m.hosted_agents_sessions_user()}</dt>
							<dd>
								{session.spec.claudeAccount === 'claude-tag'
									? 'Claude Tag · '
									: ''}{user?.displayName || user?.username || session.spec.userID}
								{#if user?.email}<span class="block text-muted-content">{user.email}</span>{/if}
							</dd>
							<dt class="text-muted-content">{m.hosted_agents_sessions_environment()}</dt>
							<dd>{session.spec.claudeEnvironment}</dd>
							<dt class="text-muted-content">{m.hosted_agents_sessions_session()}</dt>
							<dd>{session.spec.claudeSession}</dd>
						</dl>
					{/if}
					<dl class="text-sm break-all">
						<dt class="text-muted-content">{m.hosted_agents_sessions_last_used()}</dt>
						<dd>
							{#if session.status.state === 'RUNNING' && !session.spec.claudeDone}
								{m.hosted_agents_sessions_active()}
							{:else if lastUsed.relativeTime}
								<time datetime={session.spec.claudeFinishedAt} title={lastUsed.fullDate}>
									{lastUsed.relativeTime}
								</time>
							{:else}
								{m.hosted_agents_sessions_unknown()}
							{/if}
						</dd>
						<dt class="text-muted-content">{m.hosted_agents_sessions_repositories()}</dt>
						<dd>
							{#if session.spec.claudeRepositories == null}
								{m.hosted_agents_sessions_not_reported()}
							{:else if session.spec.claudeRepositories.length === 0}
								{m.hosted_agents_sessions_no_repositories()}
							{:else}
								<ul class="space-y-1">
									{#each session.spec.claudeRepositories as repository, index (index)}
										<li>
											{repository.url}
											{#if repository.revision}<span class="block text-muted-content"
													>{repository.revision}</span
												>{/if}
										</li>
									{/each}
								</ul>
							{/if}
						</dd>
					</dl>
					{#if session.spec.slackSenderName || session.spec.slackSenderHandle || session.spec.slackChannelName}
						<dl class="text-sm break-words">
							{#if session.spec.slackSenderName || session.spec.slackSenderHandle}
								<dt class="text-muted-content">{m.hosted_agents_slack_sender()}</dt>
								<dd>
									{session.spec.slackSenderName ?? ''}
									{#if session.spec.slackSenderHandle}<span class="text-muted-content"
											>@{session.spec.slackSenderHandle}</span
										>{/if}
								</dd>
							{/if}
							{#if session.spec.slackChannelName}
								<dt class="text-muted-content">{m.hosted_agents_slack_channel()}</dt>
								<dd>#{session.spec.slackChannelName}</dd>
							{/if}
						</dl>
					{/if}
					{#if session.status.error}<p class="text-error">{session.status.error}</p>{/if}
					<div class="flex flex-wrap items-center gap-4">
						{#if session.spec.slackLink && /^https:\/\/[a-z0-9-]+\.slack\.com\/archives\//i.test(session.spec.slackLink)}
							<a
								class="btn btn-ghost btn-sm"
								href={session.spec.slackLink}
								target="_blank"
								rel="external noopener noreferrer"
							>
								{m.hosted_agents_slack_thread()}
								<ExternalLink class="size-4" />
							</a>
						{/if}
						{#if !all}
							<a
								class="btn btn-ghost btn-sm self-start"
								href={`https://claude.ai/code/${encodeURIComponent(session.spec.claudeSession)}`}
								target="_blank"
								rel="noreferrer"
								>{m.hosted_agents_sessions_open()} <ExternalLink class="size-4" /></a
							>
						{/if}
						{#if profile.current.isAdmin?.() && session.status.state === 'RUNNING' && !session.spec.suspended}
							<button
								class="btn btn-ghost btn-sm"
								disabled={!!deleting || !!suspending || !!session.metadata.deletionTimestamp}
								onclick={() => (pendingSuspend = session)}
							>
								<Pause class="size-4" />{m.hosted_agents_sessions_suspend()}
							</button>
						{/if}
						<button
							class="btn btn-ghost btn-sm text-error"
							disabled={!!deleting || !!suspending || !!session.metadata.deletionTimestamp}
							onclick={() => (pendingDelete = session)}
						>
							<Trash2 class="size-4" />{deleting === session.metadata.name
								? m.hosted_agents_sessions_deleting_progress()
								: m.hosted_agents_sessions_delete()}
						</button>
					</div>
				</li>
			{:else}
				<li class="text-muted-content">
					{m.hosted_agents_sessions_empty()}
				</li>
			{/each}
		</ul>
	{/if}
</div>
<Confirm
	show={!!pendingDelete}
	title={m.hosted_agents_sessions_delete()}
	msg={m.hosted_agents_sessions_delete_question()}
	note={m.hosted_agents_sessions_delete_note()}
	submitText={m.hosted_agents_sessions_delete()}
	loading={!!deleting}
	oncancel={() => {
		if (!deleting) pendingDelete = undefined;
	}}
	onsuccess={() => {
		if (pendingDelete) void remove(pendingDelete);
	}}
/>

<Confirm
	show={!!pendingSuspend}
	title={m.hosted_agents_sessions_suspend_title()}
	msg={m.hosted_agents_sessions_suspend_question()}
	note={m.hosted_agents_sessions_suspend_note()}
	submitText={m.hosted_agents_sessions_suspend_title()}
	loading={!!suspending}
	oncancel={() => {
		if (!suspending) pendingSuspend = undefined;
	}}
	onsuccess={() => {
		if (pendingSuspend) void suspend(pendingSuspend);
	}}
/>
