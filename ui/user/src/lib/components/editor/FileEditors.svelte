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
	<div class:hidden={!file.selected} class="flex-1 overflow-auto" bind:clientHeight={height}>
		{#if file.name.toLowerCase().endsWith('.md')}
			<Milkdown {file} {onFileChanged} {onInvoke} {items} />
		{:else if file.name.toLowerCase().endsWith('.pdf')}
			<Pdf {file} {height} />
		{:else if file.table?.name}
			<Table tableName={file.table?.name} {project} {currentThreadID} {items} />
		{:else if isImage(file.name)}
			<Image {file} />
		{:else}
			<Codemirror {file} {onFileChanged} {onInvoke} {items} />
		{/if}
	</div>
{/each}
