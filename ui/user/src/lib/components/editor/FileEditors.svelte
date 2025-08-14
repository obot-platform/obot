<script lang="ts">
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import type { InvokeInput } from '$lib/services';
	import Pdf from '$lib/components/editor/Pdf.svelte';
	import { isImage } from '$lib/image';
	import Image from '$lib/components/editor/Image.svelte';
	import Codemirror from '$lib/components/editor/Codemirror.svelte';
	import MarkdownFile from './MarkdownFile.svelte';

	interface Props {
		onFileChanged: (name: string, contents: string) => void;
		onInvoke?: (invoke: InvokeInput) => void;
		items: EditorItem[];
		mdMode?: 'raw' | 'wysiwyg';
		disabled?: boolean;
		overrideContent?: string;
	}

	let height = $state<number>();
	let {
		onFileChanged,
		onInvoke,
		items = $bindable(),
		mdMode = 'wysiwyg',
		overrideContent
	}: Props = $props();
</script>

{#each items as file (file.name)}
	{#if file.name.toLowerCase().endsWith('.pdf')}
		<div
			class:hidden={!file.selected}
			class="default-scrollbar-thin h-full flex-1"
			bind:clientHeight={height}
		>
			<Pdf {file} {height} />
		</div>
	{:else}
		<div
			class:hidden={!file.selected}
			class="default-scrollbar-thin h-full flex-1"
			bind:clientHeight={height}
		>
			{#if file.name.toLowerCase().endsWith('.md')}
				<MarkdownFile
					{file}
					{onFileChanged}
					mode={mdMode}
					{onInvoke}
					{items}
					disabled={!!overrideContent}
					{overrideContent}
				/>
			{:else if isImage(file.name)}
				<Image {file} />
			{:else if [...(file?.file?.contents ?? '')].some((char) => char.charCodeAt(0) === 0)}
				{@render unsupportedFile()}
			{:else}
				<Codemirror {file} {onFileChanged} {onInvoke} {items} class="m-0 rounded-b-2xl" />
			{/if}
		</div>
	{/if}
{/each}

{#snippet unsupportedFile()}
	<div class="flex h-full w-full flex-col items-center justify-center">
		<img
			src="/user/images/obot-icon-surprised-yellow.svg"
			alt="Surprised obot"
			class="size-[200px] opacity-50"
		/>
		<p class="text-lg text-gray-500">This type of file cannot be opened in the editor</p>
	</div>
{/snippet}
