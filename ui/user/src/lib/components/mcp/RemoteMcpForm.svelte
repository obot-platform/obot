<script lang="ts">
	import type { MCPServerInfo } from '$lib/services/chat/mcp';
	import { Plus, Trash2 } from 'lucide-svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';

	interface Props {
		config: MCPServerInfo;
		showSubmitError: boolean;
		custom?: boolean;
	}
	let { config = $bindable(), showSubmitError, custom }: Props = $props();

	function focusOnAdd(node: HTMLInputElement, shouldFocus: boolean) {
		if (shouldFocus) {
			node.focus();
		}
	}
</script>

<div class="flex items-center gap-4">
	<h4 class="w-24 text-base font-semibold">URL</h4>
	<input class="text-input-filled flex grow" bind:value={config.url} />
</div>
<div class="flex flex-col gap-2">
	<h4 class="text-base font-semibold">Headers</h4>
	{#if config.headers && config.headers.length > 0}
		{#each config.headers as header, i}
			<div class="flex w-full items-center gap-2">
				<div class="flex grow flex-col gap-1">
					<input
						class="ghost-input w-full py-0 pl-1"
						bind:value={header.key}
						placeholder="Key (ex. API_KEY)"
						use:focusOnAdd={i === config.headers.length - 1}
					/>
					{#if header.sensitive}
						<SensitiveInput name={header.name} bind:value={header.value} />
					{:else}
						<input
							data-1p-ignore
							id={header.name}
							name={header.name}
							class="text-input-filled w-full"
							bind:value={header.value}
							type="text"
						/>
					{/if}
					<div class="min-h-4 text-xs text-red-500">
						{#if showSubmitError && !header.value && header.required}
							This field is required.
						{/if}
					</div>
				</div>
				{#if !header.required || custom}
					<button class="icon-button" onclick={() => config.headers?.splice(i, 1)}>
						<Trash2 class="size-4" />
					</button>
				{/if}
			</div>
		{/each}
	{/if}
	<div class="flex justify-end pt-2">
		<button
			class="button flex items-center gap-1 text-xs"
			onclick={() =>
				config.headers?.push({
					name: '',
					key: '',
					description: '',
					sensitive: false,
					required: false,
					file: false,
					value: ''
				})}
		>
			<Plus class="size-4" /> Header
		</button>
	</div>
</div>
