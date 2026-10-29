<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { deleteKill, killScopeLabel, type Kill } from '$lib/services/provenance';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import { OctagonX } from '@lucide/svelte';

	// Active kill switches, each with a revoke button for administrators.

	interface Props {
		kills: Kill[];
		onRevoked?: (id: string) => void;
	}

	let { kills, onRevoked }: Props = $props();

	let revoking = $state<string>();
	let error = $state('');
	let readonly = $derived(profile.current.isAdminReadonly?.());

	async function revoke(kill: Kill) {
		revoking = kill.id;
		error = '';
		try {
			await deleteKill(kill.id, { dontLogErrors: true });
			success.add(m.supervision_kill_revoked());
			onRevoked?.(kill.id);
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			revoking = undefined;
		}
	}
</script>

<div class="flex flex-col gap-2">
	{#if error}
		<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
	{/if}
	{#if kills.length === 0}
		<p class="text-muted-content text-sm">{m.supervision_kill_none()}</p>
	{:else}
		<ul class="flex flex-col gap-2">
			{#each kills as kill (kill.id)}
				<li class="bg-error/10 flex items-start justify-between gap-3 rounded-md p-3 text-sm">
					<div class="flex min-w-0 flex-col gap-0.5">
						<span class="flex flex-wrap items-center gap-2">
							<OctagonX class="text-error size-4 shrink-0" />
							<span class="badge badge-sm badge-error badge-soft">{killScopeLabel(kill.scope)}</span
							>
							{#if kill.target}
								<code class="truncate text-xs">{kill.target}</code>
							{/if}
						</span>
						{#if kill.reason}
							<span class="text-sm">{kill.reason}</span>
						{/if}
						<span class="text-muted-content text-xs">
							{formatTimeAgo(kill.createdAt).relativeTime}
							{#if kill.createdBy}
								{m.supervision_kill_by({ user: kill.createdBy })}
							{/if}
							{#if kill.expiresAt && !kill.expiresAt.startsWith('0001')}
								· {m.supervision_kill_expires({
									time: formatTimeUntil(kill.expiresAt).relativeTime
								})}
							{/if}
						</span>
					</div>
					{#if !readonly}
						<button
							type="button"
							class="btn btn-secondary btn-sm text-xs"
							disabled={revoking === kill.id}
							onclick={() => revoke(kill)}
						>
							{m.supervision_kill_revoke()}
						</button>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</div>
