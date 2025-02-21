<script lang="ts">
	import { Brain } from 'lucide-svelte/icons';
	import {
		ChatService,
		type Project,
		type KnowledgeFile as KnowledgeFileType
	} from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import KnowledgeUpload from '$lib/components/navbar/KnowledgeUpload.svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import KnowledgeFile from './KnowledgeFile.svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let knowledgeFiles = $state<KnowledgeFileType[]>([]);

	async function loadFiles() {
		knowledgeFiles = (await ChatService.listKnowledgeFiles(project.assistantID, project.id)).items;
	}

	let fileToDelete = $state<string>();

	async function deleteFile() {
		if (!fileToDelete) {
			return;
		}
		await ChatService.deleteKnowledgeFile(project.assistantID, project.id, fileToDelete);
		await loadFiles();
		fileToDelete = undefined;
	}
</script>

<Menu
	title="Knowledge Files"
	description="Additional knowledge the AI can search."
	onLoad={loadFiles}
>
	{#snippet icon()}
		<Brain class="h-5 w-5" />
	{/snippet}
	{#snippet body()}
		{#if knowledgeFiles.length === 0}
			<p class="p-6 text-center text-sm text-gray dark:text-gray-300">No files</p>
		{:else}
			<ul class="space-y-3 px-3 py-6 text-sm">
				{#each knowledgeFiles as file}
					<KnowledgeFile
						{file}
						onDelete={() => {
							fileToDelete = file.fileName;
						}}
					/>
				{/each}
			</ul>
		{/if}
		<KnowledgeUpload onUpload={loadFiles} {project} />
	{/snippet}
</Menu>

<Confirm
	show={fileToDelete !== undefined}
	msg={`Are you sure you want to delete ${fileToDelete}?`}
	onsuccess={deleteFile}
	oncancel={() => (fileToDelete = undefined)}
/>
