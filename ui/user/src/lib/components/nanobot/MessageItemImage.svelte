<script lang="ts">
	import type { ChatMessageItemImage } from '$lib/services/nanobot/types';
	import { isSafeImageMimeType } from '$lib/services/nanobot/utils';

	interface Props {
		item: ChatMessageItemImage;
	}

	let { item }: Props = $props();
</script>

<div class="bg-base-200 mb-3 rounded-lg p-3">
	{#if isSafeImageMimeType(item.mimeType)}
		<img src="data:{item.mimeType};base64,{item.data}" alt="" class="max-w-full rounded" />
	{:else}
		<p class="text-base-content/70 text-sm">Unsupported image type.</p>
	{/if}
</div>
