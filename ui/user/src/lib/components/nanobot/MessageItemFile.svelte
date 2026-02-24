<script lang="ts">
	import type { ChatMessageItemToolCall } from '$lib/services/nanobot/types';
	import { parseToolFilePath } from '$lib/services/nanobot/utils';
	import FileItem from '$lib/components/nanobot/FileItem.svelte';
	interface Props {
		item: ChatMessageItemToolCall;
		onFileOpen?: (filename: string) => void;
	}

	let { item, onFileOpen }: Props = $props();

	const pending = $derived(item.hasMore);
	const filename = $derived(item.arguments ? (parseToolFilePath(item) ?? '') : (item.name ?? ''));
	const name = $derived(filename ? filename.split('/').pop() : null);
</script>

<button
	class="rounded-field text border-base-300 bg-base-100 tooltip hover:bg-base-300 mt-3 mb-2 w-full border p-3 shadow-xs transition-colors"
	data-tip={`Open ${filename}`}
	onclick={() => {
		onFileOpen?.(`file:///${filename}`);
	}}
	disabled={pending}
>
	<div class="flex items-center justify-between">
		<div class="flex items-center gap-2">
			<FileItem uri={filename} compact />

			{#if pending}
				<span class="skeleton skeleton-text bg-transparent text-sm">...</span>
			{:else}
				<span class="text-sm">{name}</span>
			{/if}
		</div>
	</div>
</button>
