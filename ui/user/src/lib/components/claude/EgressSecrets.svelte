<script lang="ts">
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { m } from '$lib/i18n';
	import type { EgressSecret } from './types';
	import { Plus, Trash2 } from '@lucide/svelte';

	let { secrets = $bindable([]) }: { secrets?: EgressSecret[] } = $props();
</script>

<fieldset class="space-y-3">
	<legend class="text-sm font-semibold">{m.hosted_agents_egress_secrets()}</legend>
	<p class="text-xs text-muted-content">{m.hosted_agents_egress_secrets_help()}</p>
	{#each secrets as secret, index (index)}
		<div class="space-y-2 rounded-lg border border-base-300 p-3">
			<div class="flex items-end gap-2">
				<label class="flex min-w-0 flex-1 flex-col gap-1 text-sm">
					{m.hosted_agents_secret_hostname()}
					<input
						class="text-input-filled"
						bind:value={secret.hostname}
						required
						placeholder="api.example.com"
					/>
				</label>
				<IconButton
					type="button"
					variant="danger"
					tooltip={{ text: m.hosted_agents_remove_secret() }}
					onclick={() => (secrets = secrets.filter((_, i) => i !== index))}
					><Trash2 class="size-4" /></IconButton
				>
			</div>
			<label class="flex flex-col gap-1 text-sm">
				{m.hosted_agents_secret_header()}
				<input
					class="text-input-filled"
					bind:value={secret.header}
					required
					placeholder="Authorization"
				/>
			</label>
			<label class="flex flex-col gap-1 text-sm">
				{m.hosted_agents_secret_value()}
				<input
					class="text-input-filled"
					type="password"
					autocomplete="new-password"
					bind:value={secret.value}
					required={!secret.id}
					maxlength={8192}
					placeholder={secret.id ? m.hosted_agents_secret_keep() : 'Bearer …'}
				/>
			</label>
		</div>
	{/each}
	<button
		type="button"
		class="btn btn-ghost btn-sm"
		onclick={() => (secrets = [...secrets, { id: '', hostname: '', header: '', value: '' }])}
		><Plus class="size-4" />{m.hosted_agents_add_secret()}</button
	>
</fieldset>
