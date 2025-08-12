<script lang="ts">
	import type { InvokeInput } from '$lib/services';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import MarkdownInput from '../MarkdownInput.svelte';
	import Milkdown from './Milkdown.svelte';

	interface Props {
		file: EditorItem;
		onFileChanged: (name: string, contents: string) => void;
		onInvoke?: (invoke: InvokeInput) => void | Promise<void>;
		items: EditorItem[];
		mode: 'wysiwyg' | 'raw';
	}

	let { file: refFile, onFileChanged, mode = 'wysiwyg', onInvoke, items }: Props = $props();
	// updating of the actual file.file.contents is handled during invoke of chat
	// so have to keep a local state of the contents to share between the two variations
	let contents = $state(refFile?.file?.contents ?? '');
	let filename = $state(refFile?.name ?? '');

	$effect(() => {
		if (refFile) {
			contents = refFile.file?.contents ?? '';
			filename = refFile.name ?? '';
		}
	});

	$effect(() => {
		if (contents && contents !== refFile?.file?.contents && refFile.name === filename) {
			onFileChanged(filename, contents);
		}
	});

	function handleFileChange(_name: string, changedContent: string) {
		contents = changedContent;
	}
</script>

{#if mode === 'wysiwyg' && refFile?.file}
	<Milkdown
		file={{
			...refFile,
			file: {
				...refFile.file,
				contents
			}
		}}
		onFileChanged={handleFileChange}
		{onInvoke}
		{items}
		class="p-5 pt-0"
	/>
{:else}
	<MarkdownInput
		bind:value={contents}
		disablePreview
		class="border-surface3 h-full grow rounded-none border-0 shadow-none"
		classes={{
			input: 'bg-gray-50 h-full max-h-full'
		}}
	/>
{/if}
