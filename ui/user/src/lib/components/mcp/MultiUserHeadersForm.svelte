<script lang="ts">
	import type { MCPSubField } from '$lib/services';
	import InfoTooltip from '../InfoTooltip.svelte';
	import Toggle from '../Toggle.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import { Plus, Trash2 } from 'lucide-svelte';

	interface Props {
		headers?: MCPSubField[];
		readonly?: boolean;
	}

	let { headers = $bindable(), readonly }: Props = $props();
</script>

<div
	class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
>
	<div class="flex flex-col gap-1">
		<h4 class="text-sm font-semibold">User-Defined Headers</h4>
		<p class="text-muted-content text-xs font-light">
			These headers are collected from each user when they connect to this multi-user MCP server.
		</p>
	</div>

	{#if headers}
		{#each headers as header, i (i)}
			<div
				class="dark:border-base-400 bg-base-300 flex w-full items-center gap-4 rounded-lg border border-transparent p-4"
			>
				<div class="flex w-full flex-col gap-4">
					<div class="flex w-full flex-col gap-1">
						<label for={`multi-user-header-name-${i}`} class="text-sm font-light">Name</label>
						<input
							id={`multi-user-header-name-${i}`}
							class="text-input-filled bg-base-100 w-full shadow-none"
							bind:value={headers[i].name}
							disabled={readonly}
						/>
					</div>

					<div class="flex w-full flex-col gap-1">
						<label for={`multi-user-header-description-${i}`} class="text-sm font-light"
							>Description</label
						>
						<input
							id={`multi-user-header-description-${i}`}
							class="text-input-filled bg-base-100 w-full shadow-none"
							bind:value={headers[i].description}
							disabled={readonly}
						/>
					</div>

					<div class="flex w-full flex-col gap-1">
						<label for={`multi-user-header-key-${i}`} class="text-sm font-light">Key</label>
						<input
							id={`multi-user-header-key-${i}`}
							class="text-input-filled bg-base-100 w-full shadow-none"
							bind:value={headers[i].key}
							placeholder="e.g. X-API-Key"
							disabled={readonly}
						/>
					</div>

					<div class="flex w-full flex-col gap-1">
						<label
							for={`multi-user-header-prefix-${i}`}
							class="flex items-center gap-1 text-sm font-light"
						>
							Value Prefix
							<InfoTooltip
								text="A constant prepended value added to the user-supplied value. Example: 'Bearer '."
								popoverWidth="lg"
							/>
						</label>
						<input
							id={`multi-user-header-prefix-${i}`}
							class="text-input-filled bg-base-100 w-full shadow-none"
							bind:value={headers[i].prefix}
							disabled={readonly}
						/>
					</div>

					<div class="flex gap-8">
						<Toggle
							classes={{ label: 'text-sm text-inherit' }}
							disabled={readonly}
							label="Sensitive"
							labelInline
							checked={!!header.sensitive}
							onChange={(checked) => {
								if (headers?.[i]) headers[i].sensitive = checked;
							}}
						/>
						<Toggle
							classes={{ label: 'text-sm text-inherit' }}
							disabled={readonly}
							label="Required"
							labelInline
							checked={!!header.required}
							onChange={(checked) => {
								if (headers?.[i]) headers[i].required = checked;
							}}
						/>
					</div>
				</div>

				{#if !readonly}
					<IconButton
						class="hover:text-error"
						onclick={() => {
							headers?.splice(i, 1);
						}}
						variant="danger"
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			</div>
		{/each}
	{/if}

	{#if !readonly}
		<div class="flex justify-end">
			<button
				class="button flex items-center gap-1 text-xs"
				onclick={() => {
					if (!headers) {
						headers = [];
					}
					headers.push({
						key: '',
						description: '',
						name: '',
						value: '',
						required: false,
						sensitive: false
					});
				}}
			>
				<Plus class="size-4" />
				Header
			</button>
		</div>
	{/if}
</div>
