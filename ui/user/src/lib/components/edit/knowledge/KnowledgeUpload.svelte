<script lang="ts">
	import { Upload } from 'lucide-svelte/icons';
	import { ChatService, type Project } from '$lib/services';
	import type { KnowledgeFile } from '$lib/services';
	import Loading from '$lib/icons/Loading.svelte';
	import Error from '$lib/components/Error.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onUpload?: () => void | Promise<void>;
		project: Project;
		thread?: boolean;
		currentThreadID?: string;
		classes?: {
			button?: string;
		};
	}

	let { onUpload, project, thread, currentThreadID, classes }: Props = $props();

	let files = $state<FileList>();
	let uploadInProgress = $state<Promise<KnowledgeFile>>();

	function reloadFiles() {
		if (thread && !currentThreadID) {
			return;
		}
		ChatService.listKnowledgeFiles(project.assistantID, project.id, {
			threadID: currentThreadID
		}).then((files) => {
			const pending = files.items.find(
				(file) => file.state === 'pending' || file.state === 'ingesting'
			);
			if (pending) {
				setTimeout(reloadFiles, 2000);
			}
		});
	}

	$effect(() => {
		if (!files?.length) {
			return;
		}

		if (thread && !currentThreadID) {
			return;
		}

		uploadInProgress = ChatService.uploadKnowledge(project.assistantID, project.id, files[0], {
			threadID: currentThreadID
		});
		uploadInProgress
			.then(() => {
				onUpload?.();
			})
			.catch((error) => {
				console.error(error);
			})
			.finally(() => {
				uploadInProgress = undefined;
				setTimeout(reloadFiles, 1000);
			});

		files = undefined;
	});
</script>

<label class={twMerge('button flex items-center justify-end gap-1 text-sm', classes?.button)}>
	{#await uploadInProgress}
		<Loading class="size-4" />
	{:catch error}
		<Error {error} />
	{/await}
	{#if !uploadInProgress}
		<Upload class="size-4" />
	{/if}
	Upload
	<input
		bind:files
		type="file"
		class="hidden"
		accept=".pdf, .txt, .doc, .docx, .md, .html, .odt, .rtf, .csv, .ipynb, .json, .pptx, .ppt, .pages"
	/>
</label>
