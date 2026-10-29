<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Select from '$lib/components/Select.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		createKill,
		KILL_SCOPES,
		killScopeLabel,
		type Kill,
		type KillScope
	} from '$lib/services/provenance';
	import { success } from '$lib/stores/success';
	import { TriangleAlert } from '@lucide/svelte';

	// The kill switch denies every action at a scope on every surface. The dialog takes the scope,
	// its target (none for the organization), a reason and an optional expiry.

	interface Props {
		onCreated?: (kill: Kill) => void;
	}

	let { onCreated }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let scope = $state<KillScope>('session');
	let target = $state('');
	let reason = $state('');
	let ttlMinutes = $state<number | undefined>();
	let saving = $state(false);
	let error = $state('');

	const scopeOptions = KILL_SCOPES.map((id) => ({ id, label: killScopeLabel(id) }));

	let needsTarget = $derived(scope !== 'org');
	let valid = $derived(reason.trim() !== '' && (!needsTarget || target.trim() !== ''));

	export function open(defaults?: { scope?: KillScope; target?: string }) {
		scope = defaults?.scope ?? 'session';
		target = defaults?.target ?? '';
		reason = '';
		ttlMinutes = undefined;
		error = '';
		dialog?.open();
	}

	async function submit() {
		if (!valid || saving) return;
		saving = true;
		error = '';
		try {
			const kill = await createKill(
				{
					scope,
					target: needsTarget ? target.trim() : undefined,
					reason: reason.trim(),
					ttlSeconds: ttlMinutes && ttlMinutes > 0 ? Math.round(ttlMinutes * 60) : undefined
				},
				{ dontLogErrors: true }
			);
			success.add(m.supervision_kill_created());
			onCreated?.(kill);
			dialog?.close();
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			saving = false;
		}
	}
</script>

<ResponsiveDialog bind:this={dialog} title={m.supervision_kill_switch()} class="md:max-w-lg">
	<div class="flex flex-col gap-4">
		<p class="text-muted-content text-sm">{m.supervision_kill_switch_description()}</p>

		<div class="flex flex-col gap-1">
			<span id="kill-scope-label" class="text-sm font-light">{m.supervision_kill_scope()}</span>
			<Select
				id="kill-scope"
				class="bg-base-200 dark:bg-base-200 dark:border-base-400 border border-transparent shadow-inner"
				ariaLabelledby="kill-scope-label"
				options={scopeOptions}
				selected={scope}
				onSelect={(o) => (scope = o.id as KillScope)}
			/>
		</div>

		{#if scope === 'org'}
			<div class="notification-alert flex items-start gap-2 rounded-md p-3 text-sm">
				<TriangleAlert class="size-4 shrink-0" />
				<span>{m.supervision_kill_org_warning()}</span>
			</div>
		{:else}
			<div class="flex flex-col gap-1">
				<label for="kill-target" class="text-sm font-light">{m.supervision_kill_target()}</label>
				<input
					id="kill-target"
					class="text-input-filled"
					placeholder={m.supervision_kill_target_placeholder()}
					bind:value={target}
				/>
			</div>
		{/if}

		<div class="flex flex-col gap-1">
			<label for="kill-reason" class="text-sm font-light">{m.supervision_kill_reason()}</label>
			<textarea
				id="kill-reason"
				class="text-input-filled min-h-20"
				placeholder={m.supervision_kill_reason_placeholder()}
				bind:value={reason}></textarea>
		</div>

		<div class="flex flex-col gap-1">
			<label for="kill-ttl" class="text-sm font-light">{m.supervision_kill_ttl()}</label>
			<input
				id="kill-ttl"
				type="number"
				min="1"
				class="text-input-filled"
				bind:value={ttlMinutes}
			/>
			<span class="text-muted-content text-xs">{m.supervision_kill_ttl_help()}</span>
		</div>

		{#if error}
			<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
		{/if}

		<div class="flex justify-end gap-2">
			<button type="button" class="btn btn-secondary text-sm" onclick={() => dialog?.close()}>
				{m.common_cancel()}
			</button>
			<button
				type="button"
				class="btn btn-error text-sm"
				disabled={!valid || saving}
				onclick={submit}
			>
				{#if saving}<Loading class="size-4" />{:else}{m.supervision_kill_submit()}{/if}
			</button>
		</div>
	</div>
</ResponsiveDialog>
