<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ClaudeSessionsSettings, {
		type SessionListFilters,
		type SessionSortBy
	} from '$lib/components/claude/ClaudeSessionsSettings.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type OrgUser } from '$lib/services';
	import { doDelete, doGet, doPost } from '$lib/services/http';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import IconButton from '../primitives/IconButton.svelte';
	import { Bot, ExternalLink, Pause, Trash2 } from '@lucide/svelte';
	import { onMount } from 'svelte';

	const sortByValues: SessionSortBy[] = ['lastUsed', 'name', 'status'];
	const statusOrder = [
		'running',
		'resuming',
		'suspending',
		'pending',
		'error',
		'suspended',
		'deleting'
	];

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
	let filters = $state<SessionListFilters>({
		showMySessionsOnly: false,
		sortBy: 'lastUsed',
		query: '',
		statusFilterBy: ''
	});

	let visibleSessions = $derived.by(() => {
		let items = sessions;
		if (all && filters.showMySessionsOnly) {
			items = items.filter((session) => session.spec.userID === profile.current.id);
		}
		const query = filters.query.trim().toLowerCase();
		if (query) {
			items = items.filter((session) => sessionSearchText(session).includes(query));
		}
		const statuses = new Set(
			filters.statusFilterBy
				.split(',')
				.map((id) => id.trim())
				.filter(Boolean)
		);
		if (statuses.size > 0) {
			items = items.filter((session) => statuses.has(sessionStatusId(session)));
		}
		return [...items].sort(compareSessions);
	});

	function handleChange(property: keyof SessionListFilters, values: string[]) {
		switch (property) {
			case 'showMySessionsOnly':
				filters.showMySessionsOnly = values.includes('true');
				break;
			case 'sortBy':
				filters.sortBy = sortByValues.includes(values[0] as SessionSortBy)
					? (values[0] as SessionSortBy)
					: 'lastUsed';
				break;
			case 'query':
				filters.query = values[0] || '';
				break;
			case 'statusFilterBy':
				filters.statusFilterBy = values.join(',');
				break;
		}
	}

	function sessionSearchText(session: Session) {
		const user = users[session.spec.userID];
		return [
			session.spec.displayName,
			session.spec.userID,
			user?.displayName,
			user?.username,
			user?.email,
			session.spec.claudeEnvironment,
			session.spec.claudeSession,
			session.spec.slackSenderName,
			session.spec.slackSenderHandle,
			session.spec.slackChannelName,
			...(session.spec.claudeRepositories ?? []).flatMap((repository) => [
				repository.url,
				repository.revision
			])
		]
			.filter((part): part is string => Boolean(part))
			.join('\n')
			.toLowerCase();
	}

	function sessionStatusId(session: Session) {
		if (session.metadata.deletionTimestamp) return 'deleting';
		if (session.spec.suspended && !session.spec.claudeDone) return 'suspending';
		switch (session.status.state?.toUpperCase()) {
			case 'RUNNING':
				return 'running';
			case 'SUSPENDED':
				return 'suspended';
			case 'RESUMING':
				return 'resuming';
			case 'ERROR':
				return 'error';
			case 'PENDING':
			case undefined:
			case '':
				return 'pending';
			default:
				return session.status.state?.toLowerCase() || 'pending';
		}
	}

	function compareSessions(left: Session, right: Session) {
		if (filters.sortBy === 'name') {
			return left.spec.displayName.localeCompare(right.spec.displayName);
		}
		if (filters.sortBy === 'status') {
			const rank = (session: Session) => {
				const index = statusOrder.indexOf(sessionStatusId(session));
				return index === -1 ? statusOrder.length : index;
			};
			return (
				rank(left) - rank(right) || left.spec.displayName.localeCompare(right.spec.displayName)
			);
		}
		const recency = (session: Session) => {
			if (session.status.state?.toUpperCase() === 'RUNNING' && !session.spec.claudeDone) {
				return Number.POSITIVE_INFINITY;
			}
			const parsed = session.spec.claudeFinishedAt
				? Date.parse(session.spec.claudeFinishedAt)
				: Number.NaN;
			return Number.isNaN(parsed) ? 0 : parsed;
		};
		return (
			recency(right) - recency(left) || left.spec.displayName.localeCompare(right.spec.displayName)
		);
	}

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

<div class="flex w-full flex-col gap-4 @container">
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
		<ClaudeSessionsSettings {filters} onChange={handleChange} showMine={all} />
		{#if visibleSessions.length > 0}
			<ul class="grid grid-cols-1 gap-4 @2xl:grid-cols-2 @5xl:grid-cols-3">
				{#each visibleSessions as session (session.metadata.name)}
					{@const user = users[session.spec.userID]}
					{@const lastUsed = formatTimeAgo(session.spec.claudeFinishedAt ?? undefined)}
					<li
						class="flex min-w-0 flex-col relative group h-full text-base-content border-base-300 dark:border-base-300 bg-base-100 dark:bg-base-200 group @container gap-3 rounded-lg border p-4 shadow-xs"
					>
						<div class="flex justify-between items-center gap-4 flex-wrap">
							<h2 class="flex items-center gap-2 text-sm font-semibold">
								<Bot class="size-5 shrink-0 text-primary" />{session.spec.displayName}
							</h2>
							<p class="badge badge-soft badge-sm badge-primary" role="status">
								{session.metadata.deletionTimestamp
									? m.hosted_agents_sessions_deleting()
									: session.spec.suspended && !session.spec.claudeDone
										? m.hosted_agents_sessions_suspending()
										: sessionState(session.status.state)}
							</p>
						</div>
						{#if all}
							<dl class="text-sm break-all space-y-2">
								<div>
									<dt class="text-muted-content">{m.hosted_agents_sessions_user()}</dt>
									<dd>
										{#if session.spec.claudeAccount === 'claude-tag'}
											{m.hosted_agents_sessions_claude_tag_user({
												name: user?.displayName || user?.username || session.spec.userID
											})}
										{:else}
											{user?.displayName || user?.username || session.spec.userID}
										{/if}
										{#if user?.email}<span class="block text-muted-content">{user.email}</span>{/if}
									</dd>
								</div>
								<div>
									<dt class="text-muted-content">{m.hosted_agents_sessions_environment()}</dt>
									<dd>{session.spec.claudeEnvironment}</dd>
								</div>
								<div>
									<dt class="text-muted-content">{m.hosted_agents_sessions_session()}</dt>
									<dd>{session.spec.claudeSession}</dd>
								</div>
							</dl>
						{/if}
						<dl class="text-sm break-all space-y-2">
							<div>
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
							</div>
							<div>
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
							</div>
						</dl>
						{#if session.spec.slackSenderName || session.spec.slackSenderHandle || session.spec.slackChannelName}
							<dl class="text-sm wrap-break-word space-y-2">
								{#if session.spec.slackSenderName || session.spec.slackSenderHandle}
									<div>
										<dt class="text-muted-content">{m.hosted_agents_slack_sender()}</dt>
										<dd>
											{session.spec.slackSenderName ?? ''}
											{#if session.spec.slackSenderHandle}<span class="text-muted-content"
													>@{session.spec.slackSenderHandle}</span
												>{/if}
										</dd>
									</div>
								{/if}
								{#if session.spec.slackChannelName}
									<div>
										<dt class="text-muted-content">{m.hosted_agents_slack_channel()}</dt>
										<dd>#{session.spec.slackChannelName}</dd>
									</div>
								{/if}
							</dl>
						{/if}
						{#if session.status.error}
							<p class="alert alert-error alert-soft text-xs">
								{session.status.error}
							</p>
						{/if}
						<div class="flex grow"></div>
						<div class="flex flex-wrap items-center justify-between gap-4">
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
								{#if session.spec.userID === profile.current.id}
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
							</div>
							<IconButton
								disabled={!!deleting || !!suspending || !!session.metadata.deletionTimestamp}
								onclick={() => (pendingDelete = session)}
								class="btn-sm text-error hover:text-error-content hover:bg-error"
								tooltip={{ text: m.hosted_agents_sessions_delete() }}
							>
								{#if deleting === session.metadata.name}
									<Loading class="size-4" />
								{:else}
									<Trash2 class="size-4" />
								{/if}
							</IconButton>
						</div>
					</li>
				{/each}
			</ul>
		{:else}
			<div class="flex flex-col items-center justify-center gap-2">
				<p class="text-sm font-light text-muted-content">
					{#if sessions.length === 0}
						{m.hosted_agents_sessions_empty()}
					{:else}
						{m.hosted_agents_sessions_no_matches()}
					{/if}
				</p>
			</div>
		{/if}
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
