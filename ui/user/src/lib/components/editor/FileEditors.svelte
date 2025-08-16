<script lang="ts">
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import type { InvokeInput } from '$lib/services';
	import Pdf from '$lib/components/editor/Pdf.svelte';
	import { isImage } from '$lib/image';
	import Image from '$lib/components/editor/Image.svelte';
	import Codemirror from '$lib/components/editor/Codemirror.svelte';
	import MarkdownFile from './MarkdownFile.svelte';
	import RawEditor from './RawEditor.svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		onFileChanged: (name: string, contents: string) => void;
		onInvoke?: (invoke: InvokeInput) => void;
		items: EditorItem[];
		mdMode?: 'raw' | 'wysiwyg';
		disabled?: boolean;
		liveEditing?: {
			filename: string;
			content: string;
		};
	}

	let height = $state<number>();
	let {
		onFileChanged,
		onInvoke,
		items = $bindable(),
		mdMode = 'wysiwyg',
		liveEditing
	}: Props = $props();
	let selected = $derived(items.find((item) => item.selected));
</script>

{#if selected}
	<div class="h-full w-full" in:fade>
		{#if selected.name.toLowerCase().endsWith('.pdf')}
			<div class="default-scrollbar-thin h-full flex-1" bind:clientHeight={height}>
				<Pdf file={selected} {height} />
			</div>
		{:else}
			<div class="default-scrollbar-thin h-full flex-1" bind:clientHeight={height}>
				{#if selected.name.toLowerCase().endsWith('.md')}
					<MarkdownFile
						file={selected}
						{onFileChanged}
						mode={mdMode}
						{onInvoke}
						{items}
						disabled={!!liveEditing}
						overrideContent={liveEditing?.content}
					/>
				{:else if isImage(selected.name)}
					<Image file={selected} />
				{:else if [...(selected?.file?.contents ?? '')].some((char) => char.charCodeAt(0) === 0)}
					{@render unsupportedFile()}
				{:else}
					<Codemirror
						file={selected}
						{onFileChanged}
						{onInvoke}
						{items}
						class="m-0 rounded-b-2xl"
					/>
				{/if}
			</div>
		{/if}
	</div>
{:else if liveEditing}
	<RawEditor
		value=""
		disabled
		disablePreview
		class="border-surface3 h-full grow rounded-none border-0 bg-inherit shadow-none"
		classes={{
			input: 'bg-gray-50 h-full max-h-full pb-8 grid'
		}}
		typewriterOnAutonomous
		overrideContent={liveEditing.content}
	/>
{/if}

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
