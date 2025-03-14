<script lang="ts">
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import type { InvokeInput, Project } from '$lib/services';
	import Milkdown from '$lib/components/editor/Milkdown.svelte';
	import Pdf from '$lib/components/editor/Pdf.svelte';
	import { isImage } from '$lib/image';
	import Image from '$lib/components/editor/Image.svelte';
	import Codemirror from '$lib/components/editor/Codemirror.svelte';
	import Table from '$lib/components/editor/Table.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		onFileChanged: (name: string, contents: string) => void;
		onInvoke?: (invoke: InvokeInput) => void;
		items: EditorItem[];
	}

	let height = $state<number>();
	let { project, currentThreadID, onFileChanged, onInvoke, items = $bindable() }: Props = $props();
</script>

{#each items as file}
	<div
		class:hidden={!file.selected}
		class="default-scrollbar-thin flex-1"
		bind:clientHeight={height}
	>
		{#if file.name.toLowerCase().endsWith('.md')}
			<Milkdown {file} {onFileChanged} {onInvoke} {items} />
		{:else if file.name.toLowerCase().endsWith('.pdf')}
			<Pdf {file} {height} />
		{:else if file.table?.name}
			<Table tableName={file.table?.name} {project} {currentThreadID} {items} />
		{:else if isImage(file.name)}
			<Image {file} />
		{:else}
			{#if [...(file?.file?.contents ?? '')].some((char) => char.charCodeAt(0) === 0)}
				<div class="mx-2 mt-4 flex h-[calc(100vh-200px)] items-center justify-center border-l-2 border-gray-100 dark:border-gray-900">
					<div class="absolute flex">
						<div class="flex items-center justify-center rounded-3xl bg-gray-70 shadow-lg">
							<span class="text-lg text-gray-500">Cannot display files of this type</span>
						</div>
					</div>
				</div>
			{:else}
				<Codemirror {file} {onFileChanged} {onInvoke} {items} />
			{/if}
		{/if}
	</div>
{/each}
