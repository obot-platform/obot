<script lang="ts">
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { m } from '$lib/i18n';
	import { Plus, Trash2 } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	let {
		hosts = $bindable([]),
		label,
		classes,
		disabled = false
	}: {
		hosts?: string[];
		label: string;
		disabled?: boolean;
		classes?: {
			header?: string;
		};
	} = $props();
</script>

<fieldset {disabled} class="space-y-2">
	<legend class={twMerge('text-sm', classes?.header)}>{label}</legend>
	<p class="text-xs text-muted-content">{m.hosted_agents_egress_wildcard_help()}</p>
	{#each hosts as _, index (index)}
		<div class="flex items-center gap-2">
			<input
				class="text-input-filled min-w-0 flex-1"
				aria-label={label}
				bind:value={hosts[index]}
				placeholder={m.hosted_agents_egress_host_placeholder()}
			/>
			<IconButton
				type="button"
				variant="danger"
				tooltip={{ text: m.hosted_agents_remove_hostname() }}
				onclick={() => (hosts = hosts.filter((_, i) => i !== index))}
			>
				<Trash2 class="size-4" />
			</IconButton>
		</div>
	{/each}
	<div class="flex justify-end pt-2">
		<button type="button" class="btn btn-secondary btn-sm" onclick={() => (hosts = [...hosts, ''])}>
			<Plus class="size-4" />{m.hosted_agents_add_hostname()}
		</button>
	</div>
</fieldset>
