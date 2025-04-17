<script lang="ts">
	import type { EditorItem } from '$lib/services/editor/index.svelte';

	type Props = {
		file: EditorItem;
		height?: number | string;
	};

	const { file, height = '100%' }: Props = $props();

	let blobUrl = $state<string>();

	$effect(() => {
		if (!file.file?.blob) return;

		const url = URL.createObjectURL(new Blob([file.file?.blob], { type: 'application/pdf' }));
		blobUrl = url;

		return () => URL.revokeObjectURL(url);
	});
</script>

<div class="h-full">
	{#if blobUrl}
		<embed src={blobUrl} width="100%" {height} />
	{/if}
</div>
