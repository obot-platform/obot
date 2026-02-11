<script lang="ts">
	import type { ChatMessageItemToolCall } from '$lib/services/nanobot/types';
	import { parseToolFilePath } from '$lib/services/nanobot/utils';
	import { FileIcon, FolderIcon } from 'lucide-svelte';
	interface Props {
		item: ChatMessageItemToolCall;
		onFileOpen?: (filename: string) => void;
	}

	let { item, onFileOpen }: Props = $props();

	const pending = $derived(item.hasMore);
	const filename = $derived(item.arguments ? (parseToolFilePath(item) ?? '') : (item.name ?? ''));
	const name = $derived(filename ? filename.split('/').pop()?.split('.').shift() : null);
</script>

<div
	class="rounded-field text border-base-200 dark:border-base-300 bg-base-100 mt-3 mb-2 w-full border p-3 shadow-xs"
>
	<div class="flex items-center justify-between">
		<div class="flex items-center gap-2">
			<FileIcon class="size-4" />

			{#if pending}
				<span class="animate-pulse text-sm">...</span>
			{:else}
				<span class="text-sm">{name}</span>
			{/if}
		</div>
		<button
			class="btn btn-sm tooltip"
			data-tip="Open"
			onclick={() => {
				onFileOpen?.(`file:///${filename}`);
			}}
			disabled={pending}
		>
			<FolderIcon class="size-4" />
		</button>
	</div>
</div>
