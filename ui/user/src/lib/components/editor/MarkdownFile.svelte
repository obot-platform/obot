<script lang="ts">
	import type { InvokeInput } from '$lib/services';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import Milkdown from './Milkdown.svelte';
	import RawMarkdownEditor from './RawMarkdownEditor.svelte';

	interface Props {
		file: EditorItem;
		onFileChanged: (name: string, contents: string) => void;
		onInvoke?: (invoke: InvokeInput) => void | Promise<void>;
		items: EditorItem[];
		mode: 'wysiwyg' | 'raw';
		disabled?: boolean;
		overrideContent?: string;
	}

	let {
		file: refFile,
		onFileChanged,
		mode = 'wysiwyg',
		onInvoke,
		items,
		disabled,
		overrideContent
	}: Props = $props();
	// updating of the actual file.file.contents is handled during invoke of chat
	// so have to keep a reference of the contents to share between the two variations
	let contents = $derived(refFile?.file?.contents ?? '');
	let filename = $derived(refFile?.name ?? '');

	$effect(() => {
		if (contents && contents !== refFile?.file?.contents && refFile.name === filename) {
			onFileChanged(filename, contents);
		}
	});

	function handleFileChange(_name: string, changedContent: string) {
		if (!overrideContent) {
			contents = changedContent;
		}
	}
</script>

{#if mode === 'wysiwyg'}
	<Milkdown
		file={refFile}
		{contents}
		onFileChanged={handleFileChange}
		{overrideContent}
		{onInvoke}
		{items}
		class="p-5 pt-0"
	/>
{:else}
	<RawMarkdownEditor
		bind:value={contents}
		{disabled}
		disablePreview
		class="border-surface3 h-full grow rounded-none border-0 bg-inherit shadow-none"
		classes={{
			input: 'bg-gray-50 h-full max-h-full pb-8 grid'
		}}
		typewriterOnAutonomous
		{overrideContent}
	/>
{/if}
