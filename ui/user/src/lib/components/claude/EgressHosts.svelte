<script lang="ts">
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { m } from '$lib/i18n';
	import { Plus, Trash2 } from '@lucide/svelte';

	let {
		hosts = $bindable([]),
		label,
		disabled = false
	}: {
		hosts?: string[];
		label: string;
		disabled?: boolean;
	} = $props();
</script>

<fieldset {disabled} class="space-y-2">
	<legend class="text-sm">{label}</legend>
	<p class="text-xs text-muted-content">{m.hosted_agents_egress_wildcard_help()}</p>
	{#each hosts as _, index (index)}
		<div class="flex items-center gap-2">
			<input
				class="text-input-filled min-w-0 flex-1"
				aria-label={label}
				bind:value={hosts[index]}
				placeholder="*.github.com"
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
	<button type="button" class="btn btn-ghost btn-sm" onclick={() => (hosts = [...hosts, ''])}>
		<Plus class="size-4" />{m.hosted_agents_add_hostname()}
	</button>
</fieldset>
