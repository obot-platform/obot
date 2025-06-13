<script lang="ts">
	import { type MCPServerInfo } from '$lib/services/chat/mcp';
	import InfoTooltip from '../InfoTooltip.svelte';
	import SensitiveInput from '../SensitiveInput.svelte';

	interface Props {
		config: MCPServerInfo;
	}

	let { config = $bindable() }: Props = $props();
</script>

<div class="flex flex-col gap-2">
	{#if config.env && config.env.length > 0}
		<h4 class="text-base font-semibold">Configuration</h4>
		<div class="flex flex-col gap-6">
			{#each config.env as env, index (env.key)}
				<div class="flex flex-col gap-1">
					<label for={env.key} class="flex items-center gap-1 text-sm font-light">
						{env.required ? `${env.name || env.key}*` : `${env.name || env.key} (optional)`}
						<InfoTooltip text={env.description} />
					</label>
					{#if env.sensitive}
						<SensitiveInput name={env.name} bind:value={config.env[index].value} />
					{:else}
						<input
							data-1p-ignore
							id={env.name}
							name={env.name}
							class="text-input-filled w-full"
							bind:value={env.value}
							type="text"
						/>
					{/if}
				</div>
			{/each}
		</div>
	{/if}

	{#if config.headers && config.headers.length > 0}
		<h4 class="text-base font-semibold">Configuration</h4>
		<div class="flex flex-col gap-6">
			{#each config.headers as header, index (header.key)}
				<label for={header.key} class="flex items-center gap-1 text-sm font-light">
					{header.required
						? `${header.name || header.key}*`
						: `${header.name || header.key} (optional)`}
					<InfoTooltip text={header.description} />
				</label>
				{#if header.sensitive}
					<SensitiveInput name={header.name} bind:value={config.headers[index].value} />
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
			{/each}
		</div>
	{/if}
</div>
