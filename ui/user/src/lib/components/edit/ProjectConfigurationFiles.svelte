<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { overflowToolTip } from '$lib/actions/overflow';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import FileEditors from '$lib/components/editor/FileEditors.svelte';
	import Error from '$lib/components/Error.svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { isImage } from '$lib/image';
	import { newFileMonitor } from '$lib/save.js';
	import { ChatService, EditorService, type File, type Files, type Project } from '$lib/services';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { Download, Image, Plus } from 'lucide-svelte';
	import { FileText, Trash2, X } from 'lucide-svelte/icons';
	import { onMount } from 'svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import InfoTooltip from '../InfoTooltip.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		classes?: {
			list?: string;
		};
	}

	let { project, currentThreadID = $bindable(), classes }: Props = $props();

	const knowledgeExtensions = [
		'.pdf',
		'.txt',
		'.doc',
		'.docx',
		'.ppt',
		'.pptx',
		'.md',
		'.rtf',
		'.html',
		'.odt',
		'.ipynb',
		'.json'
	];
	const accept = knowledgeExtensions.join(', ') + ',.csv, .png, .jpg, .jpeg, .webp';
	const fileMonitor = newFileMonitor(project);
	let files = $state<File[]>([]);
	let fileToDelete = $state<string | undefined>();
	let fileList = $state<FileList>();
	let items = $state<EditorItem[]>([]);
	let editorDialog = $state<HTMLDialogElement>();
	let uploadInProgress = $state<Promise<Files>>();
	let menu = $state<ReturnType<typeof Menu>>();

	onMount(() => {
		fileMonitor.start();
		loadFiles();
	});

	$effect(() => {
		if (!fileList?.length) {
			return;
		}

		const file = fileList[0];
		uploadInProgress = ChatService.saveFile(project.assistantID, project.id, file);
		uploadInProgress.finally(() => {
			uploadInProgress = undefined;
			loadFiles();
		});

		fileList = undefined;
	});

	async function loadFiles() {
		files = (await ChatService.listFiles(project.assistantID, project.id)).items;
	}

	async function editFile(file: File) {
		await EditorService.load(items, project, file.name);
		editorDialog?.showModal();
	}

	async function deleteFile() {
		if (!fileToDelete) {
			return;
		}
		await ChatService.deleteFile(project.assistantID, project.id, fileToDelete);
		await loadFiles();
		EditorService.remove(items, fileToDelete);
		fileToDelete = undefined;
	}
</script>

<div class="flex flex-col gap-2">
	<div class="flex items-center gap-2">
		<h2 class="text-xl font-semibold">Project Files</h2>
		<InfoTooltip text={HELPER_TEXTS.projectFiles} class="size-4" classes={{ icon: 'size-4' }} />
	</div>

	<div class="flex flex-col gap-4">
		{#if files && files.length > 0}
			<ul class={classes?.list}>
				{#each files as file, i (i)}
					<li>
						<div
							class="text-md dark:bg-surface1 dark:border-surface3 flex gap-4 rounded-md border border-transparent bg-white shadow-sm"
						>
							<button
								class="flex w-4/5 flex-1 items-center gap-1 truncate p-4 text-start"
								onclick={() => editFile(file)}
							>
								{#if isImage(file.name)}
									<Image class="size-4 min-w-fit" />
								{:else}
									<FileText class="size-4 min-w-fit" />
								{/if}
								<span use:overflowToolTip>{file.name}</span>
							</button>

							<div class="mr-2 flex items-center gap-2">
								<button
									class="icon-button flex-shrink-0"
									onclick={() => {
										EditorService.download([], project, file.name);
									}}
									use:tooltip={'Download File'}
								>
									<Download class="text-gray size-5" />
								</button>

								<button
									class="icon-button flex-shrink-0"
									onclick={() => {
										fileToDelete = file.name;
										menu?.toggle(false);
									}}
									use:tooltip={'Delete File'}
								>
									<Trash2 class="text-gray size-5" />
								</button>
							</div>
						</div>
					</li>
				{/each}
			</ul>
		{/if}

		<div class="flex justify-end">
			<label class="button text-md flex cursor-pointer items-center justify-end gap-1">
				{#await uploadInProgress}
					<Loading class="size-4" />
				{:catch error}
					<Error {error} />
				{/await}
				{#if !uploadInProgress}
					<Plus class="size-4" />
				{/if}
				Add File
				<input bind:files={fileList} type="file" class="hidden" {accept} />
			</label>
		</div>
	</div>
</div>

<dialog
	bind:this={editorDialog}
	class="relative h-full max-h-dvh w-full max-w-dvw rounded-none md:w-4/5 md:rounded-xl"
	use:clickOutside={() => editorDialog?.close()}
>
	<button
		class="button-icon-primary absolute top-1 right-1 z-10"
		onclick={async () => {
			await fileMonitor.save();
			editorDialog?.close();
		}}
	>
		<X class="size-6 md:size-8" />
	</button>
	<div class="flex h-full flex-col p-5">
		{#each items as item (item.id)}
			{#if item.selected}
				<h2 class="ml-2 pr-12 text-base font-semibold break-words md:text-xl">{item.name}</h2>
			{/if}
		{/each}
		<div class="h-full overflow-y-auto">
			<FileEditors onFileChanged={fileMonitor.onFileChange} bind:items />
		</div>
	</div>
</dialog>

<Confirm
	show={fileToDelete !== undefined}
	msg={`Are you sure you want to delete ${fileToDelete}?`}
	onsuccess={deleteFile}
	oncancel={() => (fileToDelete = undefined)}
/>
