<script lang="ts">
	import { Plus, Upload } from 'lucide-svelte/icons';
	import { ChatService, type Project } from '$lib/services';
	import type { KnowledgeFile } from '$lib/services';
	import Loading from '$lib/icons/Loading.svelte';
	import Error from '$lib/components/Error.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		onUpload?: () => void | Promise<void>;
		project: Project;
		thread?: boolean;
		currentThreadID?: string;
		compact?: boolean;
	}

	let { onUpload, project, thread, currentThreadID, compact }: Props = $props();

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

{#if compact}
	<div class="flex justify-end" use:tooltip={'Upload Knowledge File'}>
		{@render content()}
	</div>
{:else}
	<div class="flex justify-end">
		{@render content()}
	</div>
{/if}

{#snippet content()}
	<label
		class={compact
			? 'icon-button cursor-pointer'
			: 'button flex items-center justify-end gap-1 text-sm'}
	>
		{#await uploadInProgress}
			<Loading class="size-5" />
		{:catch error}
			<Error {error} />
		{/await}
		{#if !uploadInProgress}
			{#if compact}
				<Plus class="size-5" />
			{:else}
				<Upload class="size-5" />
			{/if}
		{/if}
		{#if !compact}
			Upload
		{/if}
		<input
			bind:files
			type="file"
			class="hidden"
			accept=".pdf, .txt, .doc, .docx, .md, .html, .odt, .rtf, .csv, .ipynb, .json, .pptx, .ppt, .pages"
		/>
	</label>
{/snippet}
