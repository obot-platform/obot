<script lang="ts">
	import type { ChatMessageItemImage } from '$lib/services/nanobot/types';

	interface Props {
		item: ChatMessageItemImage;
	}

	let { item }: Props = $props();
</script>

<div class="bg-base-200 mb-3 rounded-lg p-3">
	<img src="data:{item.mimeType};base64,{item.data}" alt="" class="max-w-full rounded" />
</div>
