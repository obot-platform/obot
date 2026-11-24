<script lang="ts">
	import { httpErrorReason } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { Plus, TriangleAlert, X } from '@lucide/svelte';
	import { untrack } from 'svelte';

	interface Props {
		// The hosts that are saved on the agent.
		hosts: string[];
		readonly?: boolean;
		// Saves the whole list. It throws when the server refuses it.
		onSave: (hosts: string[]) => Promise<void>;
	}

	let { hosts: savedHosts, readonly, onSave }: Props = $props();

	let hosts = $state<string[]>(untrack(() => [...savedHosts]));
	let draft = $state('');
	let saving = $state(false);
	let error = $state<string>();

	const dirty = $derived(
		hosts.length !== savedHosts.length || hosts.some((host, i) => host !== savedHosts[i])
	);

	// The list follows the agent when it is saved from elsewhere, unless there are edits that aren't saved.
	$effect(() => {
		const next = [...savedHosts];
		untrack(() => {
			if (!dirty) hosts = next;
		});
	});

	function add(e?: SubmitEvent) {
		e?.preventDefault();
		const host = draft.trim();
		if (!host) return;
		if (!hosts.includes(host)) hosts = [...hosts, host];
		draft = '';
	}

	async function save() {
		// A host that was typed but not added yet is added, so it isn't lost.
		if (draft.trim()) add();
		saving = true;
		error = undefined;
		try {
			await onSave(hosts);
		} catch (e) {
			error =
				httpErrorReason(e)?.replace(/^invalid allowedReturnHosts:\s*/, '') ||
				m.identity_access_agent_profiles_failed();
		} finally {
			saving = false;
		}
	}
</script>

<section class="paper flex flex-col gap-3 p-4 text-sm" aria-labelledby="agent-return-hosts-title">
	<div class="flex flex-col gap-1">
		<h3 id="agent-return-hosts-title" class="text-lg font-semibold">
			{m.identity_access_return_hosts_title()}
		</h3>
		<p class="text-muted-content font-light">{m.identity_access_return_hosts_description()}</p>
	</div>

	{#if hosts.length === 0}
		<p class="text-warning flex items-start gap-2" role="status">
			<TriangleAlert class="size-4 shrink-0" />
			{m.identity_access_return_hosts_none_warning()}
		</p>
	{/if}

	{#if hosts.length > 0}
		<ul class="flex flex-wrap gap-2">
			{#each hosts as host (host)}
				<li class="pill-secondary flex items-center gap-1">
					<span>{host}</span>
					{#if !readonly}
						<button
							type="button"
							class="hover:text-error"
							aria-label={m.identity_access_return_hosts_remove({ host })}
							disabled={saving}
							onclick={() => (hosts = hosts.filter((candidate) => candidate !== host))}
						>
							<X class="size-3" />
						</button>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}

	{#if !readonly}
		<form class="flex flex-wrap items-end gap-2" onsubmit={add}>
			<label class="flex min-w-48 flex-1 flex-col gap-1" for="agent-return-host">
				<span class="input-label">{m.identity_access_return_hosts_label()}</span>
				<input
					id="agent-return-host"
					class="text-input-filled"
					bind:value={draft}
					placeholder={m.identity_access_return_hosts_placeholder()}
					disabled={saving}
				/>
			</label>
			<button class="btn btn-secondary text-sm" type="submit" disabled={saving || !draft.trim()}>
				<Plus class="size-4" />
				{m.identity_access_return_hosts_add()}
			</button>
		</form>
		{#if error}
			<p class="text-error text-xs" role="alert">{error}</p>
		{/if}
		<div class="flex justify-end">
			<button
				class="btn btn-primary text-sm"
				type="button"
				disabled={(!dirty && !draft.trim()) || saving}
				onclick={save}
			>
				{#if saving}
					<Loading class="size-4" />
				{:else}
					{m.identity_access_return_hosts_save()}
				{/if}
			</button>
		</div>
	{/if}
</section>
