<script lang="ts">
	import DatePicker from '$lib/components/DatePicker.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AgentIdentitiesService } from '$lib/services';
	import type { AgentAPIKeyCreateResponse } from '$lib/services/agent-identities/types';

	interface Props {
		agentId: string;
		onCreated: (key: AgentAPIKeyCreateResponse) => void;
	}

	let { agentId, onCreated }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let name = $state('');
	let description = $state('');
	let expiresAt = $state<Date | null>(null);
	let saving = $state(false);
	let showValidation = $state(false);

	let nameError = $derived(showValidation && !name.trim());

	function resetForm() {
		name = '';
		description = '';
		expiresAt = null;
		showValidation = false;
	}

	export function open() {
		resetForm();
		dialog?.open();
	}

	async function save(e?: SubmitEvent) {
		e?.preventDefault();
		showValidation = true;
		if (!name.trim()) return;

		saving = true;
		try {
			const created = await AgentIdentitiesService.createAgentAPIKey(agentId, {
				name: name.trim(),
				description: description.trim() || undefined,
				expiresAt: expiresAt?.toISOString(),
				// Keys created here can use everything the agent can. The API still accepts narrower scopes.
				canAccessLLMProxy: true,
				canAccessSkills: true,
				mcpServerIds: ['*']
			});
			dialog?.close();
			onCreated(created);
		} catch {
			// The request failed, and the error is shown by the notifications.
		} finally {
			saving = false;
		}
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	class="w-full p-4 md:max-w-xl"
	title={m.identity_access_create_api_key()}
>
	<form class="m-4 flex flex-col gap-4 text-sm font-light md:m-0" onsubmit={save}>
		<label class="flex flex-col gap-1" for="agent-key-name">
			<span class="input-label">
				{m.core_name()}
				{#if nameError}
					<span class="text-error text-xs">{m.identity_access_api_keys_name_is_required()}</span>
				{/if}
			</span>
			<input
				id="agent-key-name"
				class="text-input-filled"
				type="text"
				bind:value={name}
				disabled={saving}
			/>
		</label>

		<label class="flex flex-col gap-1" for="agent-key-description">
			<span class="input-label">{m.identity_access_api_keys_description_optional()}</span>
			<input
				id="agent-key-description"
				class="text-input-filled"
				type="text"
				bind:value={description}
				disabled={saving}
			/>
		</label>

		<div class="flex flex-col gap-1">
			<label for="agent-key-expires" class="input-label"
				>{m.identity_access_api_keys_expiration_date_optional()}</label
			>
			<DatePicker
				id="agent-key-expires"
				bind:value={expiresAt}
				onChange={(date) => (expiresAt = date)}
				placeholder={m.identity_access_api_keys_no_expiration()}
				minDate={new Date()}
			/>
		</div>

		<p class="input-description">{m.identity_access_agents_key_full_access()}</p>

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
					{m.identity_access_agents_create_key_submit()}
				{/if}
			</button>
		</div>
	</form>
</ResponsiveDialog>
