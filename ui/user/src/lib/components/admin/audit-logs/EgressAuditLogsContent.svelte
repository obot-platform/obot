<script lang="ts">
	import { columnResize } from '$lib/actions/resize';
	import { m } from '$lib/i18n';
	import { Group, UserService, type OrgUser } from '$lib/services';
	import { doGet } from '$lib/services/http';
	import { profile, responsive } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import AuditLogDetails from './AuditLogDetails.svelte';
	import { onMount } from 'svelte';
	import { SvelteURLSearchParams } from 'svelte/reactivity';

	type Entry = {
		id: string;
		createdAt: string;
		updatedAt: string;
		userID: string;
		claudeAccount: string;
		environmentID: string;
		claudeSession: string;
		actor: string;
		method: string;
		scheme: string;
		host: string;
		path: string;
		responseStatus: number;
		outcome: string;
		durationMs: number;
		requestBytes: number;
		responseBytes: number;
		requestTruncated: boolean;
		responseTruncated: boolean;
		requestHeaders?: Record<string, string[]>;
		responseHeaders?: Record<string, string[]>;
		requestBody?: string;
		responseBody?: string;
	};
	const auditor = $derived(profile.current.groups.includes(Group.AUDITOR));
	let entries = $state<Entry[]>([]);
	let total = $state(0);
	let offset = $state(0);
	let loading = $state(false);
	let error = $state('');
	let selected = $state<Entry>();
	let rightSidebar = $state<HTMLDivElement>();
	let users = $state<Record<string, OrgUser>>({});
	let environments = $state<Record<string, string>>({});
	let userID = $state('');
	let environmentID = $state('');
	let claudeAccount = $state('');
	let session = $state('');
	let host = $state('');
	let outcome = $state('');
	let start = $state('');
	let end = $state('');
	let generation = 0;
	let detailGeneration = 0;

	async function load(reset = false) {
		if (reset) offset = 0;
		const current = ++generation;
		loading = true;
		error = '';
		try {
			const params = new SvelteURLSearchParams({ limit: '50', offset: String(offset) });
			for (const [key, val] of Object.entries({
				user_id: userID,
				environment_id: environmentID,
				claude_account: claudeAccount,
				claude_session: session,
				host,
				outcome
			})) {
				if (val.trim()) params.set(key, val.trim());
			}
			if (start) params.set('start_time', new Date(start).toISOString());
			if (end) params.set('end_time', new Date(end).toISOString());
			const response = (await doGet(`/egress-audit-logs?${params}`)) as {
				items: Entry[] | null;
				total: number;
			};
			if (current !== generation) return;
			entries = response.items ?? [];
			total = response.total;
		} catch (e) {
			if (current === generation) error = e instanceof Error ? e.message : String(e);
		} finally {
			if (current === generation) loading = false;
		}
	}

	async function inspect(entry: Entry) {
		const current = ++detailGeneration;
		selected = undefined;
		rightSidebar?.showPopover();
		try {
			const detail = (await doGet(`/egress-audit-logs/${encodeURIComponent(entry.id)}`)) as Entry;
			if (current === detailGeneration) selected = detail;
		} catch (e) {
			if (current === detailGeneration) {
				error = e instanceof Error ? e.message : String(e);
				rightSidebar?.hidePopover();
			}
		}
	}

	function decode(body?: string) {
		if (body === undefined || body === null) return undefined;
		const bytes = Uint8Array.from(atob(body), (c) => c.charCodeAt(0));
		try {
			const text = new TextDecoder('utf-8', { fatal: true }).decode(bytes);
			try {
				return JSON.parse(text);
			} catch {
				return text;
			}
		} catch {
			return { encoding: 'base64', content: body };
		}
	}

	function userName(id: string) {
		return users[id]?.displayName || users[id]?.email || id;
	}

	onMount(() => {
		void load();
		void UserService.listUsersIncludeDeleted()
			.then((directory) => {
				users = Object.fromEntries(directory.map((u) => [u.id, u]));
			})
			.catch(() => {});
		void doGet('/egress-audit-logs/environments')
			.then((data) => {
				environments = Object.fromEntries(
					Object.entries(data as Record<string, string>).sort((a, b) => a[1].localeCompare(b[1]))
				);
			})
			.catch(() => {});
		return () => {
			generation++;
			detailGeneration++;
		};
	});

	function outcomeLabel(value: string) {
		switch (value) {
			case 'running':
				return m.audit_usage_egress_outcome_running();
			case 'complete':
				return m.audit_usage_egress_outcome_complete();
			case 'denied':
				return m.audit_usage_egress_outcome_denied();
			case 'error':
				return m.audit_usage_egress_outcome_error();
			case 'interrupted':
				return m.audit_usage_egress_outcome_interrupted();
			default:
				return value;
		}
	}
</script>

<div class="flex flex-col gap-4 pb-8">
	<p class="text-sm text-muted-content">
		{m.audit_usage_egress_help()}
	</p>
	<form
		class="flex flex-wrap items-end gap-3"
		onsubmit={(e) => {
			e.preventDefault();
			void load(true);
		}}
	>
		<label class="flex flex-col gap-1 text-sm"
			>{m.audit_usage_egress_user()}<select class="select" bind:value={userID}
				><option value="">{m.audit_usage_egress_all_users()}</option
				>{#each Object.values(users) as user (user.id)}<option value={user.id}
						>{userName(user.id)}</option
					>{/each}</select
			></label
		>
		<label class="flex flex-col gap-1 text-sm"
			>{m.audit_usage_egress_environment()}<select class="select" bind:value={environmentID}
				><option value="">{m.audit_usage_egress_all_environments()}</option
				>{#each Object.entries(environments) as [id, name] (id)}<option value={id}>{name}</option
					>{/each}</select
			></label
		>
		<label class="flex flex-col gap-1 text-sm"
			>{m.audit_usage_egress_account()}<input
				class="input"
				bind:value={claudeAccount}
				placeholder="user_…"
			/></label
		>
		<label class="flex flex-col gap-1 text-sm"
			>{m.audit_usage_egress_session()}<input
				class="input"
				bind:value={session}
				placeholder="session_…"
			/></label
		>
		<label class="flex flex-col gap-1 text-sm"
			>{m.audit_usage_egress_destination()}<input
				class="input"
				bind:value={host}
				placeholder="api.anthropic.com"
			/></label
		>
		<label class="flex flex-col gap-1 text-sm"
			>{m.audit_usage_egress_outcome()}<select class="select" bind:value={outcome}
				><option value="">{m.audit_usage_egress_all_outcomes()}</option
				>{#each ['running', 'complete', 'denied', 'error', 'interrupted'] as value (value)}<option
						{value}>{outcomeLabel(value)}</option
					>{/each}</select
			></label
		>
		<label class="flex flex-col gap-1 text-sm"
			>{m.audit_usage_egress_from()}<input
				type="datetime-local"
				class="input"
				bind:value={start}
			/></label
		>
		<label class="flex flex-col gap-1 text-sm"
			>{m.audit_usage_egress_to()}<input
				type="datetime-local"
				class="input"
				bind:value={end}
			/></label
		>
		<button class="btn btn-primary" disabled={loading}>{m.audit_usage_egress_apply()}</button>
		<button type="button" class="btn btn-secondary" disabled={loading} onclick={() => void load()}
			>{m.audit_usage_egress_refresh()}</button
		>
	</form>
	{#if error}<p role="alert" class="text-error">{error}</p>{/if}
	{#if loading}<p role="status">{m.audit_usage_egress_loading()}</p>{/if}
	<div class="overflow-x-auto rounded-lg border border-base-300">
		<table class="table w-full">
			<thead
				><tr
					><th>{m.audit_usage_egress_time()}</th><th>{m.audit_usage_egress_user()}</th><th
						>{m.audit_usage_egress_account()}</th
					><th>{m.audit_usage_egress_environment_session()}</th><th
						>{m.audit_usage_egress_request()}</th
					><th>{m.audit_usage_egress_response()}</th><th>{m.audit_usage_egress_duration()}</th><th
					></th></tr
				></thead
			>
			<tbody>
				{#each entries as entry (entry.id)}
					<tr>
						<td title={formatTimeAgo(entry.createdAt).fullDate}>
							{formatTimeAgo(entry.createdAt).relativeTime}
						</td>
						<td>{userName(entry.userID)}</td><td class="text-xs">{entry.claudeAccount}</td>
						<td
							><div>{environments[entry.environmentID] || entry.environmentID}</div>
							<div class="text-xs text-muted-content">{entry.claudeSession}</div></td
						>
						<td class="max-w-sm break-all">{entry.method} {entry.host}{entry.path}</td>
						<td
							>{entry.responseStatus || '—'}
							<span class="badge badge-soft">{outcomeLabel(entry.outcome)}</span></td
						>
						<td>{entry.durationMs} ms</td>
						<td
							><button class="btn btn-sm btn-secondary" onclick={() => void inspect(entry)}
								>{m.audit_usage_egress_details()}</button
							></td
						>
					</tr>
				{:else}<tr
						><td colspan="8" class="text-center text-muted-content"
							>{m.audit_usage_egress_empty()}</td
						></tr
					>{/each}
			</tbody>
		</table>
	</div>
	<div class="flex items-center gap-3">
		<button
			class="btn btn-secondary"
			disabled={loading || offset === 0}
			onclick={() => {
				offset = Math.max(0, offset - 50);
				void load();
			}}>{m.audit_usage_egress_previous()}</button
		>
		<span
			>{m.audit_usage_egress_page({
				start: total ? offset + 1 : 0,
				end: Math.min(offset + 50, total),
				total
			})}</span
		>
		<button
			class="btn btn-secondary"
			disabled={loading || offset + 50 >= total}
			onclick={() => {
				offset += 50;
				void load();
			}}>{m.audit_usage_egress_next()}</button
		>
	</div>
</div>

<div
	bind:this={rightSidebar}
	popover
	id="egress-audit-logs-details-sidebar"
	aria-label={m.audit_usage_egress_detail_title()}
	class="drawer-legacy min-w-full max-w-full md:min-w-lg md:max-w-[85vw]"
	style={!responsive.isMobile ? 'width: 32rem' : ''}
	onbeforetoggle={(event) => {
		if (event.newState === 'closed') {
			selected = undefined;
			detailGeneration++;
		}
	}}
>
	{#if selected}
		{#if !responsive.isMobile && rightSidebar}
			<div
				role="none"
				class="absolute top-0 left-0 z-30 h-full w-3 cursor-col-resize"
				use:columnResize={{ column: rightSidebar, direction: 'right' }}
			></div>
		{/if}
		<AuditLogDetails
			auditLog={{
				...selected,
				user: userName(selected.userID),
				requestID: selected.id,
				requestHeaders: auditor ? selected.requestHeaders : undefined,
				responseHeaders: auditor ? selected.responseHeaders : undefined,
				requestBody: auditor ? decode(selected.requestBody) : undefined,
				responseBody: auditor ? decode(selected.responseBody) : undefined
			}}
			shouldShowPayload
			onClose={() => rightSidebar?.hidePopover()}
		>
			{#snippet preRequestContent(entry)}
				<div class="space-y-1 p-4 text-sm">
					<p class="break-all">{entry.method} {entry.scheme}://{entry.host}{entry.path}</p>
					<p>{m.audit_usage_egress_account()}: {entry.claudeAccount}</p>
					<p>
						{m.audit_usage_egress_environment()}: {environments[entry.environmentID] ||
							entry.environmentID}
					</p>
					<p>{m.audit_usage_egress_session()}: {entry.claudeSession}</p>
					<p class="break-all">{m.audit_usage_egress_actor({ actor: entry.actor })}</p>
					<p>
						{m.audit_usage_egress_bytes({
							request: entry.requestBytes,
							response: entry.responseBytes
						})}
					</p>
					{#if entry.requestTruncated || entry.responseTruncated}<p class="text-warning">
							{entry.requestTruncated && entry.responseTruncated
								? m.audit_usage_egress_truncated_both()
								: entry.requestTruncated
									? m.audit_usage_egress_truncated_request()
									: m.audit_usage_egress_truncated_response()}
						</p>{/if}
				</div>
			{/snippet}
		</AuditLogDetails>
	{/if}
</div>
