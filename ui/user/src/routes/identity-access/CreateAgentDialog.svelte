<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AgentIdentitiesService } from '$lib/services';
	import type { AgentIdentity } from '$lib/services/agent-identities/types';

	interface Props {
		onCreated: (agent: AgentIdentity) => void;
	}

	let { onCreated }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let displayName = $state('');
	let description = $state('');
	let saving = $state(false);
	let showValidation = $state(false);

	export function open() {
		displayName = '';
		description = '';
		showValidation = false;
		dialog?.open();
	}

	async function save(e?: SubmitEvent) {
		e?.preventDefault();
		showValidation = true;
		if (!displayName.trim()) return;

		saving = true;
		try {
			// An agent has no owners: the server records the admin who creates it.
			const agent = await AgentIdentitiesService.createAgent({
				displayName: displayName.trim(),
				description: description.trim() || undefined
			});
			dialog?.close();
			onCreated(agent);
		} catch {
			// The error is shown by the notifications.
		} finally {
			saving = false;
		}
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	class="w-full p-4 md:max-w-xl"
	title={m.identity_access_create_agent_identity()}
>
	<form class="m-4 flex flex-col gap-4 text-sm font-light md:m-0" onsubmit={save}>
		<label class="flex flex-col gap-1" for="create-agent-name">
			<span class="input-label">
				{m.core_name()}
				{#if showValidation && !displayName.trim()}
					<span class="text-error text-xs">{m.identity_access_api_keys_name_is_required()}</span>
				{/if}
			</span>
			<input
				id="create-agent-name"
				class="text-input-filled"
				type="text"
				bind:value={displayName}
				disabled={saving}
			/>
		</label>
		<label class="flex flex-col gap-1" for="create-agent-description">
			<span class="input-label">{m.identity_access_api_keys_description_optional()}</span>
			<input
				id="create-agent-description"
				class="text-input-filled"
				type="text"
				bind:value={description}
				placeholder={m.identity_access_agents_description_placeholder()}
				disabled={saving}
			/>
		</label>
		<div class="mt-2 flex flex-col justify-end gap-2 md:flex-row">
			<button
				class="btn btn-secondary"
				type="button"
				disabled={saving}
				onclick={() => dialog?.close()}
			>
				{m.common_cancel()}
			</button>
			<button class="btn btn-primary" type="submit" disabled={saving}>
				{#if saving}
					<Loading class="size-4" />
				{:else}
					{m.core_save()}
				{/if}
			</button>
		</div>
	</form>
</ResponsiveDialog>
