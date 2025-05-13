<script lang="ts">
	import type { MCPServerInfo } from '$lib/services/chat/mcp';
	import { Plus, Trash2 } from 'lucide-svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
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

{#if config.env}
	<div class="flex flex-col gap-1">
		<h4 class="text-base font-semibold">Environment Variables</h4>
		{#each config.env as env, i}
			<div class="flex w-full items-center gap-2">
				<div class="flex grow flex-col gap-1">
					{#if !env.required}
						<input
							class="ghost-input w-full py-0"
							bind:value={env.key}
							placeholder="Key (ex. API_KEY)"
							use:focusOnAdd={i === config.env.length - 1}
						/>
					{:else}
						<label for={env.name} class="flex items-center gap-1 text-sm font-light">
							{env.required ? `${env.name ?? env.key}*` : `${env.name ?? env.key} (optional)`}
							<InfoTooltip text={env.description} />
						</label>
					{/if}
					{#if env.sensitive}
						<SensitiveInput name={env.name} bind:value={env.value} />
					{:else}
						<input
							data-1p-ignore
							id={env.name}
							name={env.name}
							class="text-input-filled w-full"
							class:error={showSubmitError && !env.value && env.required}
							bind:value={env.value}
							type="text"
						/>
					{/if}

					<div class="min-h-4 text-xs text-red-500">
						{#if showSubmitError && !env.value && env.required}
							This field is required.
						{/if}
					</div>
				</div>
				{#if !env.required || custom}
					<button class="icon-button" onclick={() => config.env?.splice(i, 1)}>
						<Trash2 class="size-4" />
					</button>
				{/if}
			</div>
		{/each}
		<div class="flex justify-end">
			<button
				class="button flex items-center gap-1 text-xs"
				onclick={() =>
					config.env?.push({
						name: '',
						key: '',
						description: '',
						sensitive: false,
						required: false,
						file: false,
						value: ''
					})}
			>
				<Plus class="size-4" /> Environment Variable
			</button>
		</div>
	</div>
{/if}

<div class="flex items-center gap-4">
	<h4 class="text-base font-semibold">Command</h4>
	<input class="text-input-filled w-full" bind:value={config.command} />
</div>

{#if config.args}
	<div class="flex gap-4">
		<h4 class="mt-1.5 text-base font-semibold">Arguments</h4>
		<div class="flex grow flex-col gap-4">
			{#each config.args as _arg, i}
				<div class="flex items-center gap-2">
					<input class="text-input-filled w-full" bind:value={config.args[i]} />
					<button class="icon-button" onclick={() => config.args?.splice(i, 1)}>
						<Trash2 class="size-4" />
					</button>
				</div>
			{/each}

			<div class="flex justify-end">
				<button
					class="button flex items-center gap-1 text-xs"
					onclick={() => config.args?.push('')}
				>
					<Plus class="size-4" /> Argument
				</button>
			</div>
		</div>
	</div>
{/if}
