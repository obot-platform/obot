<script lang="ts">
	import type { MCPConfigurationOption } from '$lib/services';
	import IconButton from '../primitives/IconButton.svelte';
	import { Plus, Trash2 } from '@lucide/svelte';

	interface Props {
		options?: MCPConfigurationOption[];
		readonly?: boolean;
	}

	let { options = $bindable(), readonly }: Props = $props();

	function addOption() {
		options ??= [];
		options.push({ value: '', name: '', description: '' });
	}

	function removeOption(index: number) {
		options?.splice(index, 1);
		if (!options?.length) options = undefined;
	}
</script>

<div class="flex flex-col gap-2">
	<div class="flex items-center justify-between">
		<div>
			<h5 class="text-sm font-medium">Selectable Values</h5>
			<p class="text-muted-content text-xs font-light">
				When provided, users choose one of these values instead of entering free-form text.
			</p>
		</div>
		{#if !readonly}
			<button type="button" class="btn btn-secondary btn-sm" onclick={addOption}>
				<Plus class="size-4" />
				Option
			</button>
		{/if}
	</div>

	{#each options ?? [] as option, index (index)}
		<div class="border-base-400 bg-base-100 grid gap-2 rounded-lg border p-3 md:grid-cols-3">
			<label class="flex flex-col gap-1 text-xs font-light">
				Name
				<input class="text-input-filled shadow-none" bind:value={option.name} disabled={readonly} />
			</label>
			<label class="flex flex-col gap-1 text-xs font-light">
				Value
				<input
					class="text-input-filled shadow-none"
					bind:value={option.value}
					disabled={readonly}
				/>
			</label>
			<div class="flex items-end gap-2">
				<label class="flex grow flex-col gap-1 text-xs font-light">
					Description <span class="text-muted-content">(optional)</span>
					<input
						class="text-input-filled shadow-none"
						bind:value={option.description}
						disabled={readonly}
					/>
				</label>
				{#if !readonly}
					<IconButton
						variant="danger2"
						onclick={() => removeOption(index)}
						tooltip={{ text: 'Delete Option' }}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			</div>
		</div>
	{/each}
</div>
