<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Truncate from '$lib/components/shared/tooltip/Truncate.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { type KnowledgeFile } from '$lib/services';
	import { CircleX, FileText, Trash } from 'lucide-svelte/icons';

	interface Props {
		onDelete?: () => void;
		file: KnowledgeFile;
	}

	const { onDelete, file }: Props = $props();

	let truncateEl = $state<ReturnType<typeof Truncate>>();
	let isError = $derived(file.state === 'error' || file.state === 'failed');
</script>

<div class="space-between group flex gap-2">
	<button
		class="flex flex-1 items-center"
		use:tooltip={{
			disabled: !isError && !truncateEl?.truncated,
			text: isError ? (file.error ?? 'Failed') : file.fileName,
			className: isError ? 'bg-red-600' : ''
		}}
	>
		<FileText class="size-5 min-w-fit" />
		<Truncate class="ms-3" text={file.fileName} disabled bind:this={truncateEl} />
		{#if file.state === 'error' || file.state === 'failed'}
			<CircleX class="ms-2 h-4 text-red-600" />
		{:else if file.state === 'pending' || file.state === 'ingesting'}
			<Loading class="mx-1.5" />
		{/if}
	</button>

	<button
		class="hidden group-hover:block"
		onclick={() => {
			if (file.state === 'ingested') {
				onDelete?.();
			}
		}}
	>
		<Trash class="h-5 w-5 text-gray" />
	</button>
</div>
