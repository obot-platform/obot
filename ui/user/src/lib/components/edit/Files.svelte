<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { overflowToolTip } from '$lib/actions/overflow';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import FileEditors from '$lib/components/editor/FileEditors.svelte';
	import Error from '$lib/components/Error.svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { getLayout } from '$lib/context/chatLayout.svelte';
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
	import { Download, Image, Plus } from 'lucide-svelte';
	import { FileText, Trash2, Upload, X } from 'lucide-svelte/icons';
	import { onMount } from 'svelte';
	import CollapsePane from './CollapsePane.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';

	interface Props {
		project: Project;
		thread?: boolean;
		currentThreadID?: string;
		primary?: boolean;
		helperText?: string;
		classes?: {
			list?: string;
		};
	}

	let {
		project,
		currentThreadID = $bindable(),
		thread = false,
		primary = true,
		helperText = '',
		classes
	}: Props = $props();

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

	onMount(() => {
		if (!thread) {
			fileMonitor.start();
			loadFiles();
		}
	});

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

{#snippet content()}
	{#if files && files.length > 0}
		<ul class={classes?.list}>
			{#each files as file}
				<li class="group">
					<div class="flex">
						<button
							class="flex w-4/5 flex-1 items-center gap-1 truncate text-start"
							onclick={() => editFile(file)}
						>
							{#if isImage(file.name)}
								<Image class="size-4 min-w-fit" />
							{:else}
								<FileText class="size-4 min-w-fit" />
							{/if}
							<span use:overflowToolTip>{file.name}</span>
						</button>

						<button
							class="icon-button-small ms-2 opacity-0 transition-all duration-200 group-hover:opacity-100"
							onclick={() => {
								EditorService.download([], project, file.name, apiOpts);
							}}
						>
							<Download class="text-gray size-4" />
						</button>

						<button
							class="icon-button-small ms-2 opacity-0 transition-all duration-200 group-hover:opacity-100"
							onclick={() => {
								fileToDelete = file.name;
								menu?.toggle(false);
							}}
						>
							<Trash2 class="text-gray size-4" />
						</button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
	{#if thread}
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
	{/if}
{/snippet}

{#snippet menuBody()}
	{#if thread}
		<Menu
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
			{#snippet body()}
				{@render content()}
			{/snippet}
			{#snippet icon()}
				<FileText class="h-5 w-5" />
			{/snippet}
		</Menu>
	{:else}
		<CollapsePane
			classes={{ header: 'pl-3 py-2', content: 'p-2' }}
			iconSize={5}
			header="Project Files"
			helpText={HELPER_TEXTS.projectFiles}
		>
			<div class="flex flex-col gap-4">
				{@render content()}
				<div class="flex justify-end">
					<label class="button flex cursor-pointer items-center justify-end gap-1 text-xs">
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
		</CollapsePane>
	{/if}
{/snippet}

{#if helperText}
	<div use:tooltip={helperText}>
		{@render menuBody()}
	</div>
{:else}
	{@render menuBody()}
{/if}

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
		{#each items as item}
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
