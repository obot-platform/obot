<script lang="ts">
	import type { MCPCatalogServer } from '$lib/services';
	import type { MCPCatalogEntry } from '$lib/services/admin/types';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		class?: string;
		entry: MCPCatalogEntry | MCPCatalogServer;
	}

	let { entry, class: klass }: Props = $props();
</script>

<div class={twMerge('flex flex-col gap-4', klass)}>
	{#if 'manifest' in entry}
		<p>{entry.manifest.description}</p>
	{:else}
		{@const manifest = entry.commandManifest || entry.urlManifest}
		{#if manifest}
			<p>{manifest.description}</p>
		{/if}
	{/if}

	{@render capabilities()}
	{@render tools()}
	{@render details()}
</div>

{#snippet capabilities()}
	<div class="flex flex-col gap-2">
		<h4 class="text-md font-semibold">Capabilities</h4>
	</div>
{/snippet}

{#snippet tools()}
	<div class="flex flex-col gap-2">
		<h4 class="text-md font-semibold">Tools</h4>
	</div>
{/snippet}

{#snippet details()}
	<div class="flex flex-col gap-2">
		<h4 class="text-md font-semibold">Details</h4>
	</div>
{/snippet}
