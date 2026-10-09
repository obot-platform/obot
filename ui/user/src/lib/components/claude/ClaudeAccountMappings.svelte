<script lang="ts">
	import Select from '$lib/components/Select.svelte';
	import { m } from '$lib/i18n';
	import { UserService, type OrgUser } from '$lib/services';
	import { doGet, doPut } from '$lib/services/http';
	import type { ClaudeEnvironment, ClaudeOrganizationAccount } from './types';
	import { onMount } from 'svelte';

	let {
		organization,
		environments,
		onsaved
	}: {
		organization: string;
		environments: Pick<
			ClaudeEnvironment,
			'id' | 'displayName' | 'organizationID' | 'environmentID'
		>[];
		onsaved: () => Promise<void>;
	} = $props();
	type Row = ClaudeOrganizationAccount & { source: string };
	let saved = $state<ClaudeOrganizationAccount[]>([]);
	let users = $state<OrgUser[]>([]);
	let rows = $state<Row[]>([]);
	let json = $state('');
	let reviewedJSON = $state('');
	let editing = $state(false);
	let ready = $state(false);
	let busy = $state(false);
	let error = $state('');
	let notice = $state('');
	const url = $derived(
		`https://claude.ai/api/organizations/${encodeURIComponent(organization)}/members`
	);
	const endpoint = $derived(`/claude-organizations/${encodeURIComponent(organization)}/accounts`);
	const options = $derived([
		{ id: '', label: m.hosted_agents_map_unmapped() },
		...users.map((user) => ({
			id: user.id,
			label: `${user.displayName || user.username} (${user.email})`
		}))
	]);
	const duplicates = $derived.by(() => {
		const selected = rows.map((row) => row.userID).filter(Boolean);
		return new Set(selected).size !== selected.length;
	});

	async function load() {
		const [accounts, directory] = await Promise.all([doGet(endpoint), UserService.listUsers()]);
		saved = (accounts as { items: ClaudeOrganizationAccount[] }).items;
		users = directory.filter(
			(user) => !user.deletedAt && !(user.internal && user.username?.startsWith('claude-tag:'))
		);
		rows = saved.map((account) => ({ ...account, source: m.hosted_agents_map_saved() }));
		ready = true;
	}
	onMount(() => {
		void load().catch((e) => (error = String(e)));
	});

	function preview() {
		error = '';
		notice = '';
		try {
			if (json.length > 4 * 1024 * 1024) throw new Error(m.hosted_agents_map_size_error());
			const members: unknown = JSON.parse(json);
			if (!Array.isArray(members) || members.length === 0 || members.length > 10000)
				throw new Error(m.hosted_agents_map_array_error());
			// eslint-disable-next-line svelte/prefer-svelte-reactivity -- Local validation state, never rendered.
			const seen = new Set<string>();
			const imported = members.map((member): Row => {
				const account = member?.account;
				if (!account || typeof account !== 'object')
					throw new Error(m.hosted_agents_map_account_error());
				const id = account.tagged_id || account.uuid;
				if (
					typeof id !== 'string' ||
					!/^(user_[A-Za-z0-9]{1,120}|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$/.test(
						id
					)
				)
					throw new Error(m.hosted_agents_map_id_error());
				for (const field of ['uuid', 'full_name', 'email_address']) {
					if (account[field] !== undefined && typeof account[field] !== 'string')
						throw new Error(m.hosted_agents_map_field_error({ field }));
				}
				const previous = saved.find(
					(row) =>
						row.account === id ||
						(account.uuid && row.uuid.toLowerCase() === account.uuid.toLowerCase())
				);
				const key = previous?.account ?? id;
				if (seen.has(key)) throw new Error(m.hosted_agents_map_duplicate_error());
				seen.add(key);
				const email = (account.email_address ?? '').trim();
				const matches = email
					? users.filter((user) => user.email?.trim().toLowerCase() === email.toLowerCase())
					: [];
				return {
					organizationID: organization,
					account: key,
					uuid: account.uuid ?? previous?.uuid ?? '',
					displayName: account.full_name ?? '',
					email,
					userID: previous?.userID || (matches.length === 1 ? matches[0].id : ''),
					source: previous?.userID
						? m.hosted_agents_map_saved()
						: matches.length === 1
							? m.hosted_agents_map_email()
							: matches.length > 1
								? m.hosted_agents_map_ambiguous()
								: m.hosted_agents_map_no_match()
				};
			});
			reviewedJSON = json;
			rows = [
				...imported,
				...saved
					.filter((row) => !seen.has(row.account))
					.map((row) => ({ ...row, source: m.hosted_agents_map_absent() }))
			];
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	async function submit() {
		busy = true;
		error = '';
		notice = '';
		try {
			const response = await doPut(endpoint, {
				items: rows.map(({ source: _source, ...account }) => account)
			});
			saved = (response as { items: ClaudeOrganizationAccount[] }).items;
			rows = saved.map((account) => ({ ...account, source: m.hosted_agents_map_saved() }));
			json = '';
			reviewedJSON = '';
			await onsaved();
			notice = m.hosted_agents_map_saved_notice();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}
</script>

<section
	class="space-y-4 rounded-xl border border-base-300 bg-base-100 p-5 shadow-sm"
	aria-label={m.hosted_agents_map_for({ organization })}
>
	<div class="flex flex-wrap items-start justify-between gap-3">
		<div>
			<h2 class="font-semibold">{m.hosted_agents_map_title()}</h2>
			<p class="text-sm text-muted-content">
				{m.hosted_agents_map_organization()} <code>{organization}</code>
			</p>
			<p class="text-sm text-muted-content">
				{m.hosted_agents_map_used_by({
					names: environments.map((environment) => environment.displayName).join(', ')
				})}
			</p>
		</div>
		<button
			class="btn btn-ghost btn-sm"
			disabled={busy || !ready}
			onclick={() => {
				editing = !editing;
				notice = '';
			}}>{editing ? m.hosted_agents_map_close() : m.hosted_agents_map_manage()}</button
		>
	</div>
	{#if error}<p role="alert" class="text-error">{error}</p>{/if}
	{#if notice}<p role="status" class="text-success">{notice}</p>{/if}
	{#if editing}
		<ol class="list-decimal space-y-2 pl-5 text-sm">
			<li>{m.hosted_agents_map_signin()}</li>
			<li>
				{m.hosted_agents_map_copy_json()}
				<a
					class="link break-all"
					href={`https://claude.ai/api/organizations/${encodeURIComponent(organization)}/members`}
					target="_blank"
					rel="noreferrer">{url}</a
				>
			</li>
			<li>
				{m.hosted_agents_map_instructions()}
			</li>
		</ol>
		<label class="block text-sm font-light"
			>{m.hosted_agents_map_json()}<textarea
				class="text-input-filled mt-1 min-h-32 w-full font-mono"
				bind:value={json}
				disabled={busy}
				placeholder={m.hosted_agents_map_json_placeholder()}></textarea></label
		>
		<button class="btn btn-ghost btn-sm" disabled={busy || !json.trim()} onclick={preview}
			>{m.hosted_agents_map_preview()}</button
		>
		<p class="text-sm text-muted-content">
			{m.hosted_agents_mapping_help()}
		</p>
		{#if rows.length}
			<div class="overflow-x-auto">
				<table class="table w-full">
					<thead
						><tr
							><th>{m.hosted_agents_map_member()}</th><th>{m.hosted_agents_sessions_user()}</th><th
								>{m.hosted_agents_map_match()}</th
							></tr
						></thead
					>
					<tbody>
						{#each rows as row (row.account)}
							<tr>
								<td class="max-w-72 break-words"
									><div class="font-medium">{row.displayName || row.email || row.account}</div>
									<div class="text-sm text-muted-content">{row.email}</div>
									<code class="text-xs">{row.account}</code></td
								>
								<td class="min-w-64">
									<span id={`mapping-${organization}-${row.account}`} class="sr-only"
										>{m.hosted_agents_map_user_for({ account: row.email || row.account })}</span
									>
									<Select
										id={`user-${organization}-${row.account}`}
										ariaLabelledby={`mapping-${organization}-${row.account}`}
										{options}
										selected={row.userID}
										placeholder={m.hosted_agents_map_unmapped()}
										searchInDropdown
										disabled={busy}
										onSelect={(option) => {
											row.userID = String(option.id);
											row.source = m.hosted_agents_map_manual();
										}}
									/>
									{#if row.userID && !users.some((user) => user.id === row.userID)}<p
											class="text-error text-sm"
										>
											{m.hosted_agents_map_unavailable({ id: row.userID })}
										</p>{/if}
								</td>
								<td class="text-sm text-muted-content">{row.source}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
			{#if duplicates}<p role="alert" class="text-error">
					{m.hosted_agents_map_duplicate_selection()}
				</p>{/if}
			<button
				class="btn btn-primary btn-sm"
				disabled={busy ||
					json !== reviewedJSON ||
					duplicates ||
					rows.some((row) => row.userID && !users.some((user) => user.id === row.userID))}
				onclick={submit}
				>{busy ? m.hosted_agents_map_saving() : m.hosted_agents_map_submit()}</button
			>
		{:else}<p class="text-sm text-muted-content">{m.hosted_agents_map_empty()}</p>{/if}
	{/if}
</section>
