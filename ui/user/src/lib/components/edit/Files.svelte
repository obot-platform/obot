<script lang="ts">
	import { overflowToolTip } from '$lib/actions/overflow';
	import Confirm from '$lib/components/Confirm.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import FileEditors from '$lib/components/editor/FileEditors.svelte';
	import Error from '$lib/components/Error.svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { isImage } from '$lib/image';
	import { newFileMonitor } from '$lib/save.js';
	import {
		ChatService,
		EditorService,
		type File,
		type Files,
		type Project,
		type Thread
	} from '$lib/services';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { responsive } from '$lib/stores';
	import { Download, Image } from 'lucide-svelte';
	import { FileText, Trash, Upload, X } from 'lucide-svelte/icons';
	import { onMount } from 'svelte';

	interface Props {
		project: Project;
		thread?: boolean;
		currentThreadID?: string;
		primary?: boolean;
	}

	let { project, currentThreadID = $bindable(), thread = false, primary = true }: Props = $props();

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
	const layout = getLayout();
	const fileMonitor = newFileMonitor(project);
	let files = $state<File[]>([]);
	let fileToDelete = $state<string | undefined>();
	let fileList = $state<FileList>();
	let items = $state<EditorItem[]>([]);
	let editorDialog = $state<HTMLDialogElement>();
	let apiOpts = $derived(thread ? { threadID: currentThreadID } : {});
	let uploadInProgress = $state<Promise<Files>>();
	let menu = $state<ReturnType<typeof Menu>>();

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
				if (isKnowledgeFile(file.name) && thread && currentThreadID) {
					return ChatService.uploadKnowledge(project.assistantID, project.id, file, apiOpts);
				}
			})
			.finally(() => {
				uploadInProgress = undefined;
				loadFiles();
			});

		fileList = undefined;
	});

	function isKnowledgeFile(name: string): boolean {
		return knowledgeExtensions.some((ext) => name.toLowerCase().endsWith(ext));
	}

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
			menu?.toggle(false);
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
		if (isKnowledgeFile(fileToDelete) && thread && currentThreadID) {
			await ChatService.deleteKnowledgeFile(project.assistantID, project.id, fileToDelete, apiOpts);
		}
		EditorService.remove(items, fileToDelete);
		fileToDelete = undefined;
	}
</script>

{#snippet body()}
	{#if files.length === 0}
		<p class="text-gray pt-6 pb-3 text-center text-sm font-light dark:text-gray-300">No files</p>
	{:else}
		<ul class="max-h-[60vh] space-y-4 overflow-y-auto py-6 ps-3 text-sm">
			{#each files as file}
				<li class="group">
					<div class="flex">
						<button
							class="flex w-4/5 flex-1 items-center truncate text-start"
							onclick={() => editFile(file)}
						>
							{#if isImage(file.name)}
								<Image class="size-5 min-w-fit" />
							{:else}
								<FileText class="size-5 min-w-fit" />
							{/if}
							<span use:overflowToolTip>{file.name}</span>
						</button>

						<button
							class="icon-button-small invisible ms-2 group-hover:visible"
							onclick={() => {
								EditorService.download([], project, file.name, apiOpts);
							}}
						>
							<Download class="text-gray h-5 w-5" />
						</button>

						<button
							class="icon-button-small invisible ms-2 group-hover:visible"
							onclick={() => {
								fileToDelete = file.name;
								menu?.toggle(false);
							}}
						>
							<Trash class="text-gray h-5 w-5" />
						</button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}

	<div class="flex justify-end">
		<label class="button mt-3 -mr-3 -mb-3 flex items-center justify-end gap-1 text-sm">
			{#await uploadInProgress}
				<Loading class="size-4" />
			{:catch error}
				<Error {error} />
			{/await}
			{#if !uploadInProgress}
				<Upload class="size-4" />
			{/if}
			Upload
			<input bind:files={fileList} type="file" class="hidden" {accept} />
		</label>
	</div>
{/snippet}

{#if thread}
	<Menu
		{body}
		bind:this={menu}
		title="Files"
		description="Content available to AI."
		onLoad={loadFiles}
		classes={{
			button: primary ? 'button-icon-primary' : '',
			dialog: responsive.isMobile
				? 'rounded-none max-h-[calc(100vh-64px)] left-0 bottom-0 w-full'
				: ''
		}}
		slide={responsive.isMobile ? 'up' : undefined}
		fixed={responsive.isMobile}
	>
		{#snippet icon()}
			<FileText class="h-5 w-5" />
		{/snippet}
	</Menu>
{:else}
	<CollapsePane header="Files" onOpen={loadFiles}>
		{@render body()}
	</CollapsePane>
{/if}

<dialog bind:this={editorDialog} class="relative h-full w-full md:w-4/5">
	<button
		class="button-icon-primary absolute top-2 right-2"
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

<style>
	.group:hover .group-hover\:visible {
		visibility: visible;
	}
</style>
