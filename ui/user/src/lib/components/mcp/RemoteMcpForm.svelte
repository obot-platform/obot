<script lang="ts">
	import type { MCPCatalogEntryFormData } from '$lib/services/admin/types';

	interface Props {
		config: MCPCatalogEntryFormData;
		readonly?: boolean;
	}
	let { config = $bindable(), readonly }: Props = $props();
</script>

<div
	class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
>
	<h4 class="w-24 text-sm font-light">URL</h4>
	<input class="text-input-filled flex grow" bind:value={config.fixedURL} disabled={readonly} />
</div>
