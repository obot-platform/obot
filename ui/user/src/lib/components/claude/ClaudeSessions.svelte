<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import ClaudeSessionsSettings, {
		SESSION_SORT_OPTIONS,
		SESSION_STATUS_FILTER_OPTIONS,
		type SessionListFilters,
		type SessionListVariant,
		type SessionSortBy,
		type SessionSortDirection
	} from '$lib/components/claude/ClaudeSessionsSettings.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type ClaudeSession, type OrgUser } from '$lib/services';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { setFilterUrlParams, setSortUrlParams } from '$lib/url';
	import InfoTooltip from '../InfoTooltip.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import { Bot, CircleAlert, Ellipsis, ExternalLink, Pause, Trash2 } from '@lucide/svelte';
	import { onMount } from 'svelte';

	const sortByValues: SessionSortBy[] = SESSION_SORT_OPTIONS.map(
		(option) => option.id as SessionSortBy
	);
	const variantValues: SessionListVariant[] = ['grid', 'table'];
	const statusOrder = SESSION_STATUS_FILTER_OPTIONS.map((option) => option.id);

	let { all = false }: { all?: boolean } = $props();

	type Session = ClaudeSession;
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
	let filters = $state(getInitialFilters());

	let listedSessions = $derived.by(() => {
		if (all && filters.showMySessionsOnly) {
			return sessions.filter((session) => session.spec.userID === profile.current.id);
		}
		return sessions;
	});

	let visibleSessions = $derived.by(() => {
		let items = listedSessions;
		const query = filters.query.trim().toLowerCase();
		if (query) {
			items = items.filter((session) => sessionSearchText(session).includes(query));
		}
		const statuses = new Set(parseSelectedFilterIds(filters.statusFilterBy));
		if (statuses.size > 0) {
			items = items.filter((session) => statuses.has(sessionStatusId(session)));
		}
		const userIds = new Set(parseSelectedFilterIds(filters.userFilterBy));
		if (userIds.size > 0) {
			items = items.filter((session) => userIds.has(session.spec.userID));
		}
		return [...items].sort(compareSessions);
	});

	function getInitialFilters(): SessionListFilters {
		const urlSortBy = page.url.searchParams.get('sort');
		const urlDirection = page.url.searchParams.get('sortDirection');
		const urlVariant = page.url.searchParams.get('variant');
		let sortBy: SessionSortBy = sortByValues.includes(urlSortBy as SessionSortBy)
			? (urlSortBy as SessionSortBy)
			: 'lastUsed';
		if (!all && sortBy === 'user') sortBy = 'lastUsed';
		const sortDirection: SessionSortDirection = urlDirection === 'asc' ? 'asc' : 'desc';
		return {
			showMySessionsOnly: page.url.searchParams.get('showMySessionsOnly') === 'true',
			sortBy,
			sortDirection,
			query: page.url.searchParams.get('query') || '',
			statusFilterBy: page.url.searchParams.get('status') || '',
			userFilterBy: all ? page.url.searchParams.get('user') || '' : '',
			variant: variantValues.includes(urlVariant as SessionListVariant)
				? (urlVariant as SessionListVariant)
				: 'grid'
		};
	}

	function parseSelectedFilterIds(selected: string) {
		return selected
			.split(',')
			.map((id) => id.trim())
			.filter(Boolean);
	}

	function handleTableSort(property: string, direction: SessionSortDirection) {
		if (!sortByValues.includes(property as SessionSortBy)) return;
		if (!all && property === 'user') return;
		filters.sortBy = property as SessionSortBy;
		filters.sortDirection = direction;
		setSortUrlParams(property, direction);
	}

	function handleChange(property: keyof SessionListFilters, values: string[]) {
		switch (property) {
			case 'showMySessionsOnly':
				filters.showMySessionsOnly = values.includes('true');
				setFilterUrlParams(property, values);
				break;
			case 'sortBy': {
				const sortBy = sortByValues.includes(values[0] as SessionSortBy)
					? (values[0] as SessionSortBy)
					: 'lastUsed';
				filters.sortBy = !all && sortBy === 'user' ? 'lastUsed' : sortBy;
				filters.sortDirection = filters.sortBy === 'lastUsed' ? 'desc' : 'asc';
				setSortUrlParams(filters.sortBy, filters.sortDirection);
				break;
			}
			case 'sortDirection':
				filters.sortDirection = values[0] === 'asc' ? 'asc' : 'desc';
				setFilterUrlParams('sortDirection', [filters.sortDirection]);
				break;
			case 'query':
				filters.query = values[0] || '';
				setFilterUrlParams('query', filters.query ? [filters.query] : []);
				break;
			case 'statusFilterBy':
				filters.statusFilterBy = values.join(',');
				setFilterUrlParams('status', values);
				break;
			case 'userFilterBy':
				filters.userFilterBy = values.join(',');
				setFilterUrlParams('user', values);
				break;
			case 'variant':
				filters.variant = variantValues.includes(values[0] as SessionListVariant)
					? (values[0] as SessionListVariant)
					: 'grid';
				setFilterUrlParams('variant', values);
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

	function statusRank(session: Session) {
		const index = statusOrder.indexOf(sessionStatusId(session));
		return index === -1 ? statusOrder.length : index;
	}

	function sessionRecency(session: Session) {
		if (session.status.state?.toUpperCase() === 'RUNNING' && !session.spec.claudeDone) {
			return Number.POSITIVE_INFINITY;
		}
		const parsed = session.spec.claudeFinishedAt
			? Date.parse(session.spec.claudeFinishedAt)
			: Number.NaN;
		return Number.isNaN(parsed) ? 0 : parsed;
	}

	function compareSessions(left: Session, right: Session) {
		const nameTie = left.spec.displayName.localeCompare(right.spec.displayName);
		let primary = nameTie;
		if (filters.sortBy === 'status') primary = statusRank(left) - statusRank(right);
		else if (filters.sortBy === 'user') primary = userText(left).localeCompare(userText(right));
		else if (filters.sortBy === 'lastUsed') primary = sessionRecency(left) - sessionRecency(right);
		const directed = filters.sortDirection === 'asc' ? primary : -primary;
		return directed || nameTie;
	}

	async function refresh() {
		if (refreshing) return;
		refreshing = true;
		try {
			sessions = await UserService.listClaudeSessions({ all, dontLogErrors: true });
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
			await UserService.suspendClaudeSession(session.metadata.name);
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
			await UserService.deleteClaudeSession(session.metadata.name);
			pendingDelete = undefined;
			await refresh();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			deleting = '';
		}
	}

	function statusLabelFromId(id: string) {
		return SESSION_STATUS_FILTER_OPTIONS.find((option) => option.id === id)?.label ?? id;
	}

	function statusLabel(session: Session) {
		return statusLabelFromId(sessionStatusId(session));
	}

	function userLabelFromId(id: string) {
		return userFilterOptions.find((option) => option.id === id)?.label ?? id;
	}

	function slackThreadLink(session: Session) {
		const link = session.spec.slackLink;
		return link && /^https:\/\/[a-z0-9-]+\.slack\.com\/archives\//i.test(link) ? link : '';
	}

	function repositoriesText(session: Session) {
		const repositories = session.spec.claudeRepositories;
		if (repositories == null) return m.hosted_agents_sessions_not_reported();
		if (repositories.length === 0) return m.hosted_agents_sessions_no_repositories();
		return repositories
			.map((repository) =>
				repository.revision ? `${repository.url} (${repository.revision})` : repository.url
			)
			.join(', ');
	}

	function slackText(session: Session) {
		const sender = [
			session.spec.slackSenderName,
			session.spec.slackSenderHandle ? `@${session.spec.slackSenderHandle}` : ''
		]
			.filter(Boolean)
			.join(' ');
		const channel = session.spec.slackChannelName ? `#${session.spec.slackChannelName}` : '';
		return [sender, channel].filter(Boolean).join(' · ') || '—';
	}

	function userText(session: Session) {
		const user = users[session.spec.userID];
		const name = user?.displayName || user?.username || session.spec.userID;
		const label =
			session.spec.claudeAccount === 'claude-tag'
				? m.hosted_agents_sessions_claude_tag_user({ name })
				: name;
		return user?.email ? `${label} (${user.email})` : label;
	}

	function lastUsedLabel(session: Session) {
		if (session.status.state === 'RUNNING' && !session.spec.claudeDone) {
			return { text: m.hosted_agents_sessions_active(), at: '' };
		}
		const formatted = formatTimeAgo(session.spec.claudeFinishedAt);
		if (!formatted.relativeTime) return { text: m.hosted_agents_sessions_unknown(), at: '' };
		return { text: formatted.relativeTime, at: session.spec.claudeFinishedAt ?? '' };
	}

	function claudeUrl(session: Session) {
		return `https://claude.ai/code/${encodeURIComponent(session.spec.claudeSession)}`;
	}

	function canOpenClaude(session: Session) {
		return session.spec.userID === profile.current.id;
	}

	function canSuspend(session: Session) {
		return Boolean(
			profile.current.isAdmin?.() && session.status.state === 'RUNNING' && !session.spec.suspended
		);
	}

	function actionsDisabled(session: Session) {
		return Boolean(deleting || suspending || session.metadata.deletionTimestamp);
	}

	type SessionRow = {
		id: string;
		session: Session;
		displayName: string;
		status: string;
		statusId: string;
		user: string;
		userId: string;
		environment: string;
		claudeSession: string;
		repositories: string;
		slack: string;
		lastUsed: string;
		lastUsedAt: string;
	};

	function toSessionRow(session: Session): SessionRow {
		const lastUsed = lastUsedLabel(session);
		return {
			id: session.metadata.name,
			session,
			displayName: session.spec.displayName,
			status: statusLabel(session),
			statusId: sessionStatusId(session),
			user: userText(session),
			userId: session.spec.userID,
			environment: session.spec.claudeEnvironment,
			claudeSession: session.spec.claudeSession,
			repositories: repositoriesText(session),
			slack: slackText(session),
			lastUsed: lastUsed.text,
			lastUsedAt: lastUsed.at
		};
	}

	let userFilterOptions = $derived.by(() => {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const seen = new Set<string>();
		const options: { id: string; label: string }[] = [];
		for (const session of listedSessions) {
			const id = session.spec.userID;
			if (!id || seen.has(id)) continue;
			seen.add(id);
			options.push({ id, label: userText(session) });
		}
		return options.sort((left, right) =>
			left.label.localeCompare(right.label, undefined, { sensitivity: 'base' })
		);
	});

	let tableRows = $derived(visibleSessions.map(toSessionRow));
	let filterTableRows = $derived(listedSessions.map(toSessionRow));
	let tableSortable = $derived(
		all ? ['lastUsed', 'displayName', 'user', 'status'] : ['lastUsed', 'displayName', 'status']
	);
	let tableFilterable = $derived(all ? ['status', 'user'] : ['status']);
	let tableColumnFilters = $derived.by(() => {
		const next: Record<string, string[]> = {};
		const statuses = parseSelectedFilterIds(filters.statusFilterBy);
		const userIds = parseSelectedFilterIds(filters.userFilterBy);
		if (statuses.length > 0) next.status = statuses;
		if (all && userIds.length > 0) next.user = userIds;
		return Object.keys(next).length > 0 ? next : undefined;
	});

	let tableFields = $derived(
		all
			? [
					'displayName',
					'user',
					'status',
					'environment',
					'claudeSession',
					'repositories',
					'slack',
					'lastUsed'
				]
			: ['displayName', 'status', 'repositories', 'slack', 'lastUsed']
	);

	let tableHeaders = $derived([
		{ title: m.core_name(), property: 'displayName' },
		{ title: m.core_status(), property: 'status' },
		...(all
			? [
					{ title: m.hosted_agents_sessions_user(), property: 'user' },
					{ title: m.hosted_agents_sessions_environment(), property: 'environment' },
					{ title: m.hosted_agents_sessions_session(), property: 'claudeSession' }
				]
			: []),
		{ title: m.hosted_agents_sessions_repositories(), property: 'repositories' },
		{ title: m.hosted_agents_sessions_slack(), property: 'slack' },
		{ title: m.hosted_agents_sessions_last_used(), property: 'lastUsed' }
	]);
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
		<ClaudeSessionsSettings {filters} onChange={handleChange} showMine={all} {userFilterOptions} />

		{#if visibleSessions.length > 0}
			{#if filters.variant === 'table'}
				{@render sessionsTable()}
			{:else}
				<ul class="grid grid-cols-1 gap-4 @2xl:grid-cols-2 @5xl:grid-cols-3">
					{#each visibleSessions as session (session.metadata.name)}
						{@const user = users[session.spec.userID]}
						{@const lastUsed = formatTimeAgo(session.spec.claudeFinishedAt ?? undefined)}
						{@const slackLink = slackThreadLink(session)}
						<li
							class="flex min-w-0 flex-col relative group h-full text-base-content border-base-300 dark:border-base-300 bg-base-100 dark:bg-base-200 group @container gap-3 rounded-lg border p-4 shadow-xs"
						>
							<div class="flex justify-between items-center gap-4 flex-wrap">
								<h2 class="flex items-center gap-2 text-sm font-semibold">
									<Bot class="size-5 shrink-0 text-primary" />{session.spec.displayName}
								</h2>
								<p class="badge badge-soft badge-sm badge-primary" role="status">
									{statusLabel(session)}
								</p>
							</div>
							{#if all}
								<dl class="break-all space-y-2 text-xs">
									<div class="flex items-center gap-2 flex-wrap">
										<dt class="font-medium w-24">{m.hosted_agents_sessions_user()}</dt>
										<dd>
											{#if session.spec.claudeAccount === 'claude-tag'}
												{m.hosted_agents_sessions_claude_tag_user({
													name: user?.displayName || user?.username || session.spec.userID
												})}
											{:else}
												{user?.displayName || user?.username || session.spec.userID}
											{/if}
											{#if user?.email}<span class="block text-muted-content">{user.email}</span
												>{/if}
										</dd>
									</div>
									<div class="flex items-center gap-2 flex-wrap">
										<dt class="font-medium w-24">{m.hosted_agents_sessions_environment()}</dt>
										<dd>{session.spec.claudeEnvironment}</dd>
									</div>
									<div class="flex items-center gap-2 flex-wrap">
										<dt class="font-medium w-24">{m.hosted_agents_sessions_session()}</dt>
										<dd>{session.spec.claudeSession}</dd>
									</div>
								</dl>
							{/if}
							<dl class="space-y-1 border border-base-300 dark:border-base-400 rounded-lg text-xs">
								<dt
									class="font-medium text-uppercase px-3 py-2 border-b border-base-300 dark:border-base-400 rounded-t-lg"
								>
									{m.hosted_agents_sessions_repositories()}
								</dt>
								<dd>
									{#if session.spec.claudeRepositories == null}
										<span class="block text-muted-content p-3">
											{m.hosted_agents_sessions_not_reported()}
										</span>
									{:else if session.spec.claudeRepositories.length === 0}
										<span class="block text-muted-content p-3">
											{m.hosted_agents_sessions_no_repositories()}
										</span>
									{:else}
										<ul class="space-y-1 p-3">
											{#each session.spec.claudeRepositories as repository, index (index)}
												<li class="truncate break-all">
													{repository.url}
													{#if repository.revision}
														<span
															class="block text-muted-content text-[11px] font-light truncate break-all"
														>
															{repository.revision}
														</span>
													{/if}
												</li>
											{/each}
										</ul>
									{/if}
								</dd>
							</dl>
							{#if session.spec.slackSenderName || session.spec.slackSenderHandle || session.spec.slackChannelName || slackLink}
								<dl
									class="wrap-break-word rounded-lg border border-base-300 dark:border-base-400 p-3 text-xs flex items-start gap-4"
								>
									<img
										src="/user/images/slack-mark/slack-mark.svg"
										alt="Slack"
										class="h-6 shrink-0 mt-1"
									/>
									<div class="flex items-center gap-4 grow flex-wrap">
										{#if session.spec.slackSenderName || session.spec.slackSenderHandle}
											<div>
												<dt class="font-medium">{m.hosted_agents_slack_sender()}</dt>
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
												<dt class="font-medium">{m.hosted_agents_slack_channel()}</dt>
												<dd>#{session.spec.slackChannelName}</dd>
											</div>
										{/if}
									</div>
									{#if slackLink}
										<a
											class="btn btn-square btn-ghost btn-sm"
											href={slackLink}
											target="_blank"
											rel="external noopener noreferrer"
											aria-label={m.hosted_agents_slack_thread()}
											use:tooltip={{ text: m.hosted_agents_slack_thread() }}
										>
											<ExternalLink class="size-4" />
										</a>
									{/if}
								</dl>
							{/if}
							{#if session.status.error}
								<p class="alert alert-error alert-soft text-xs">
									{session.status.error}
								</p>
							{/if}
							<div class="flex grow"></div>
							{#if session.spec.userID === profile.current.id || (profile.current.isAdmin?.() && session.status.state === 'RUNNING' && !session.spec.suspended)}
								<div class="flex flex-wrap items-center gap-4">
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
							{/if}
							<div class="flex flex-wrap items-baseline justify-between gap-4">
								<dl class="flex items-center gap-1 flex-wrap text-xs text-muted-content font-light">
									<dt>{m.hosted_agents_sessions_last_used()}:</dt>
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
								</dl>
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
			{/if}
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

{#snippet sessionsTable()}
	<div class="bg-base-100 dark:bg-base-300 rounded-md shadow-sm">
		<Table
			data={tableRows}
			filterData={filterTableRows}
			manualSorting
			fields={tableFields}
			headers={tableHeaders}
			noDataMessage={m.hosted_agents_sessions_empty()}
			noAutoHideFields={tableFields}
			columnMaxWidths={{
				displayName: 240,
				user: 200,
				environment: 160,
				claudeSession: 180,
				repositories: 280,
				slack: 220
			}}
			classes={{ root: 'rounded-md shadow-none' }}
			setRowClasses={() => 'group'}
			filterable={tableFilterable}
			sortable={tableSortable}
			filterValue={(property, row) => {
				if (property === 'status') return row.statusId;
				if (property === 'user') return row.userId;
				return undefined;
			}}
			displayValue={(property, value) =>
				property === 'status'
					? statusLabelFromId(String(value ?? ''))
					: property === 'user'
						? userLabelFromId(String(value ?? ''))
						: String(value ?? '')}
			initSort={{ property: filters.sortBy, order: filters.sortDirection }}
			filters={tableColumnFilters}
			onSort={handleTableSort}
			onFilter={(property, values) => {
				if (property === 'status') handleChange('statusFilterBy', values);
				else if (property === 'user') handleChange('userFilterBy', values);
			}}
			onClearAllFilters={() => {
				handleChange('statusFilterBy', []);
				if (all) handleChange('userFilterBy', []);
			}}
		>
			{#snippet onRenderColumn(property, row)}
				{#if property === 'displayName'}
					<div class="flex min-w-0 items-center gap-2">
						<p class="truncate" title={row.displayName}>{row.displayName}</p>
						{#if row.session.status.error}
							<InfoTooltip
								text={row.session.status.error}
								icon={CircleAlert}
								classes={{ icon: 'text-error' }}
							/>
						{/if}
					</div>
				{:else if property === 'status'}
					<p class="badge badge-soft badge-sm badge-primary" role="status">{row.status}</p>
				{:else if property === 'lastUsed'}
					{#if row.lastUsedAt}
						<time datetime={row.lastUsedAt}>{row.lastUsed}</time>
					{:else}
						{row.lastUsed}
					{/if}
				{:else}
					{@const text = row[property as keyof typeof row]}
					{#if typeof text === 'string'}
						<p class="truncate" title={text}>{text}</p>
					{/if}
				{/if}
			{/snippet}

			{#snippet actions(row)}
				{@const session = row.session}
				{@const slackLink = slackThreadLink(session)}
				<DotDotDot
					class="hover:dark:bg-base-100/50"
					classes={{ menu: 'min-w-48' }}
					ariaLabel={m.hosted_agents_sessions_actions_for_named({ name: session.spec.displayName })}
				>
					{#snippet icon()}
						<Ellipsis class="size-4" />
					{/snippet}
					{#snippet children({ toggle })}
						{#if canOpenClaude(session)}
							<a
								class="menu-button"
								href={claudeUrl(session)}
								target="_blank"
								rel="external noopener noreferrer"
								onclick={(e) => {
									e.stopPropagation();
									toggle(false);
								}}
							>
								<ExternalLink class="size-4" />
								{m.hosted_agents_sessions_open()}
							</a>
						{/if}
						{#if slackLink}
							<a
								class="menu-button"
								href={slackLink}
								target="_blank"
								rel="external noopener noreferrer"
								onclick={(e) => {
									e.stopPropagation();
									toggle(false);
								}}
							>
								<ExternalLink class="size-4" />
								{m.hosted_agents_slack_thread()}
							</a>
						{/if}
						{#if canSuspend(session)}
							<button
								class="menu-button"
								disabled={actionsDisabled(session)}
								onclick={(e) => {
									e.stopPropagation();
									pendingSuspend = session;
									toggle(false);
								}}
							>
								<Pause class="size-4" />
								{m.hosted_agents_sessions_suspend()}
							</button>
						{/if}
						<button
							class="menu-button-destructive"
							disabled={actionsDisabled(session)}
							onclick={(e) => {
								e.stopPropagation();
								pendingDelete = session;
								toggle(false);
							}}
						>
							{#if deleting === session.metadata.name}
								<Loading class="size-4" />
							{:else}
								<Trash2 class="size-4" />
							{/if}
							{m.hosted_agents_sessions_delete()}
						</button>
					{/snippet}
				</DotDotDot>
			{/snippet}
		</Table>
	</div>
{/snippet}
