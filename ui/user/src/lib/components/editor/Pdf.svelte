<script lang="ts">
	interface EditorItem {
		id: string;
		name: string;
		file?: {
			contents: string;
			modified?: boolean;
			buffer: string;
			threadID?: string;
			blob?: Blob;
			taskID?: string;
			runID?: string;
		};

		selected?: boolean;
		generic?: boolean;
	}

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
