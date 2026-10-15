<script lang="ts">
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { m } from '$lib/i18n';
	import type { EgressSecret } from './types';
	import { Plus, Trash2 } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	let {
		secrets = $bindable([]),
		classes
	}: { secrets?: EgressSecret[]; classes?: { header?: string; root?: string } } = $props();
</script>

<fieldset class={twMerge('space-y-3 block paper', classes?.root)}>
	<h3 class={twMerge('font-semibold', classes?.header)}>{m.hosted_agents_egress_secrets()}</h3>
	<p class="text-xs text-muted-content">{m.hosted_agents_egress_secrets_help()}</p>
	<p class="text-xs text-muted-content">
		{m.hosted_agents_env_changes_help()}
	</p>
	{#each secrets as secret, index (index)}
		<div
			class="dark:border-base-400 bg-base-200 dark:bg-base-300 space-y-2 w-full items-start gap-4 rounded-lg border border-transparent p-4"
		>
			<div class="flex items-end gap-2">
				<label class="flex min-w-0 flex-1 flex-col gap-1 text-sm">
					{m.hosted_agents_secret_hostname()}
					<input
						class="text-input-filled"
						bind:value={secret.hostname}
						required
						placeholder={m.hosted_agents_secret_hostname_placeholder()}
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
					placeholder={m.hosted_agents_secret_header_placeholder()}
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
					placeholder={secret.id
						? m.hosted_agents_secret_keep()
						: m.hosted_agents_secret_value_placeholder()}
				/>
			</label>
		</div>
	{/each}
	<div class="flex justify-end">
		<button
			type="button"
			class="btn btn-secondary btn-sm"
			onclick={() => (secrets = [...secrets, { id: '', hostname: '', header: '', value: '' }])}
			><Plus class="size-4" />{m.hosted_agents_add_secret()}</button
		>
	</div>
</fieldset>
