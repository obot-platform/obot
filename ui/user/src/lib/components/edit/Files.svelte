<script lang="ts">
	import { FileText, Trash, Upload, X } from 'lucide-svelte/icons';
	import {
		ChatService,
		EditorService,
		type File,
		type Files,
		type Project,
		type Thread
	} from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import { Download, Image } from 'lucide-svelte';
	import { isImage } from '$lib/image';
	import Error from '$lib/components/Error.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import FileEditors from '$lib/components/editor/FileEditors.svelte';
	import { newFileMonitor } from '$lib/save.js';
	import { onMount } from 'svelte';
	import { overflowToolTip } from '$lib/actions/overflow';
	import { popover } from '$lib/actions';

	interface Props {
		project: Project;
		thread?: boolean;
		currentThreadID?: string;
	}

	let { project, currentThreadID = $bindable(), thread = false }: Props = $props();

	const layout = getLayout();
	const fileMonitor = newFileMonitor(project);
	let files = $state<File[]>([]);
	let fileToDelete = $state<string | undefined>();
	let fileList = $state<FileList>();
	let items = $state<EditorItem[]>([]);
	let editorDialog = $state<HTMLDialogElement>();
	let apiOpts = $derived(
		thread
			? {
					threadID: currentThreadID
				}
			: {}
	);
	let uploadInProgress = $state<Promise<Files>>();
	let threadTT = popover();

	if (!thread) {
		onMount(() => fileMonitor.start());
	}

	$effect(() => {
		if (!fileList?.length) {
			return;
		}

		if (thread && !currentThreadID) {
			createThread()
				.then((t) => {
					currentThreadID = t.id;
				})
				.catch(() => {
					fileList = undefined;
				});
			return;
		}

		const file = fileList[0];
		uploadInProgress = ChatService.saveFile(project.assistantID, project.id, file, apiOpts);
		uploadInProgress
			.then(() => {
				if (file.name.endsWith('.pdf') && thread && currentThreadID) {
					return ChatService.uploadKnowledge(project.assistantID, project.id, file, apiOpts);
				}
			})
			.finally(() => {
				uploadInProgress = undefined;
				loadFiles();
			});

		fileList = undefined;
	});

	async function sleep(ms: number): Promise<void> {
		return new Promise((resolve) => setTimeout(resolve, ms));
	}

	async function createThread(): Promise<Thread> {
		let thread = await ChatService.createThread(project.assistantID, project.id);
		while (!thread.ready) {
			await sleep(1000);
			thread = await ChatService.getThread(project.assistantID, project.id, thread.id);
		}
		return thread;
	}

	async function loadFiles() {
		files = (await ChatService.listFiles(project.assistantID, project.id, apiOpts)).items;
	}

	async function editFile(file: File) {
		if (thread) {
			await EditorService.load(layout.items, project, file.name, apiOpts);
			layout.fileEditorOpen = true;
			layout.sidebarOpen = false;
		} else {
			await EditorService.load(items, project, file.name, apiOpts);
			editorDialog?.showModal();
		}
	}

	async function deleteFile() {
		if (!fileToDelete) {
			return;
		}
		await ChatService.deleteFile(project.assistantID, project.id, fileToDelete, apiOpts);
		await loadFiles();
		if (fileToDelete.endsWith('.pdf') && thread && currentThreadID) {
			await ChatService.deleteKnowledgeFile(project.assistantID, project.id, fileToDelete, apiOpts);
		}
		EditorService.remove(items, fileToDelete);
		fileToDelete = undefined;
	}
</script>

{#snippet body()}
	{#if files.length === 0}
		<p class="pb-3 pt-6 text-center text-sm text-gray dark:text-gray-300">No files</p>
	{:else}
		<ul class="max-h-[60vh] space-y-4 overflow-y-auto px-3 py-6 text-sm">
			{#each files as file}
				<li class="group">
					<div class="flex">
						<button
							class="flex w-4/5 flex-1 items-center text-start"
							onclick={async () => {
								await editFile(file);
							}}
						>
							{#if isImage(file.name)}
								<Image class="size-5 min-w-fit" />
							{:else}
								<FileText class="size-5 min-w-fit" />
							{/if}
							<span use:overflowToolTip>{file.name}</span>
						</button>
						<button
							class="ms-2 hidden group-hover:block"
							onclick={() => {
								EditorService.download([], project, file.name, apiOpts);
							}}
						>
							<Download class="h-5 w-5 text-gray" />
						</button>
						<button
							class="ms-2 hidden group-hover:block"
							onclick={() => {
								fileToDelete = file.name;
							}}
						>
							<Trash class="h-5 w-5 text-gray" />
						</button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}

	<div class="flex justify-end">
		<label
			class="-mb-3 -mr-3 mt-3 flex cursor-pointer justify-end gap-2 rounded-3xl p-3 px-4 hover:bg-gray-500 hover:text-white"
		>
			Upload
			<input bind:files={fileList} type="file" class="hidden" />
			{#await uploadInProgress}
				<Loading class="h-5 w-5" />
			{:catch error}
				<Error {error} />
			{/await}
			{#if !uploadInProgress}
				<Upload class="h-5 w-5" />
			{/if}
		</label>
	</div>
{/snippet}

{#if thread}
	<button
		use:threadTT.ref
		class="icon-button button-icon-primary"
		onclick={() => {
			threadTT.toggle();
			loadFiles();
		}}
	>
		<FileText class="h-5 w-5" />
	</button>
	<div use:threadTT.tooltip id="foo" class="default-dialog hidden w-[400px]">
		<div class="flex flex-col p-5">
			{@render body()}
		</div>
	</div>
{:else}
	<CollapsePane header="Files" onOpen={loadFiles}>
		{@render body()}
	</CollapsePane>
{/if}

<dialog bind:this={editorDialog} class="relative h-full w-full md:w-4/5">
	<button
		class="button-icon-primary absolute right-2 top-2"
		onclick={async () => {
			await fileMonitor.save();
			editorDialog?.close();
		}}
	>
		<X class="icon-default" />
	</button>
	<div class="flex h-full flex-col p-5">
		{#each items as item}
			{#if item.selected}
				<h2 class="ml-2 text-2xl font-semibold">{item.name}</h2>
			{/if}
		{/each}
		<div class="overflow-y-auto rounded-lg">
			<FileEditors {project} onFileChanged={fileMonitor.onFileChange} bind:items />
		</div>
	</div>
</dialog>

<Confirm
	show={fileToDelete !== undefined}
	msg={`Are you sure you want to delete ${fileToDelete}?`}
	onsuccess={deleteFile}
	oncancel={() => (fileToDelete = undefined)}
/>
