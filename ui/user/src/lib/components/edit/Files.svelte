<script lang="ts">
	import { FileText, Trash, Upload } from 'lucide-svelte/icons';
	import { ChatService, EditorService, type File, type Files, type Project } from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { Download, Image } from 'lucide-svelte';
	import { isImage } from '$lib/image';
	import Error from '$lib/components/Error.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { getLayout } from '$lib/context/layout.svelte';

	interface Props {
		project: Project;
		thread?: boolean;
		currentThreadID?: string;
		items: EditorItem[];
	}

	async function loadFiles() {
		files = (await ChatService.listFiles(project.assistantID, project.id, apiOpts)).items;
	}

	async function deleteFile() {
		if (!fileToDelete) {
			return;
		}
		await ChatService.deleteFile(project.assistantID, project.id, fileToDelete, apiOpts);
		await loadFiles();
		fileToDelete = undefined;
	}

	let { project, currentThreadID, thread = false, items = $bindable() }: Props = $props();

	const layout = getLayout();
	let files = $state<File[]>([]);
	let fileToDelete = $state<string | undefined>();
	let menu = $state<ReturnType<typeof Menu>>();
	let fileList = $state<FileList>();
	let apiOpts = $derived({
		threadID: currentThreadID
	});

	let uploadInProgress = $state<Promise<Files>>();

	$effect(() => {
		if (!fileList?.length) {
			return;
		}

		uploadInProgress = ChatService.saveFile(project.assistantID, project.id, fileList[0], apiOpts);
		uploadInProgress.finally(() => {
			uploadInProgress = undefined;
			loadFiles();
		});

		fileList = undefined;
	});
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
							class="flex flex-1 items-center"
							onclick={async () => {
								await EditorService.load(items, project, file.name, apiOpts);
								layout.fileEditorOpen = true;
								menu?.toggle(false);
							}}
						>
							{#if isImage(file.name)}
								<Image class="h-5 w-5" />
							{:else}
								<FileText class="h-5 w-5" />
							{/if}
							<span class="ms-3"
								>{file.name.length > 25 ? file.name.slice(0, 25) + '...' : file.name}</span
							>
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
	<Menu
		bind:this={menu}
		title="Files"
		description="Click to view or edit files"
		onLoad={loadFiles}
		{body}
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

<Confirm
	show={fileToDelete !== undefined}
	msg={`Are you sure you want to delete ${fileToDelete}?`}
	onsuccess={deleteFile}
	oncancel={() => (fileToDelete = undefined)}
/>
