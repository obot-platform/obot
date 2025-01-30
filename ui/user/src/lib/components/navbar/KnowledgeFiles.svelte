<script lang="ts">
	import { Brain } from 'lucide-svelte/icons';
	import { knowledgeFiles } from '$lib/stores';
	import { ChatService } from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import KnowledgeFile from './KnowledgeFile.svelte';
	import KnowledgeUpload from '$lib/components/navbar/KnowledgeUpload.svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';

	async function loadFiles() {
		knowledgeFiles.items = (await ChatService.listKnowledgeFiles()).items;
	}

	let fileToDelete = $state<string | undefined>();

	async function deleteFile() {
		if (!fileToDelete) {
			return;
		}
		await ChatService.deleteKnowledgeFile(fileToDelete);
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
		{#if knowledgeFiles.items.length === 0}
			<p class="p-6 text-center text-sm text-gray dark:text-gray-300">No files</p>
		{:else}
			<ul class="space-y-3 px-3 py-6 text-sm">
				{#each knowledgeFiles.items as file}
					<li>
						<KnowledgeFile
							{file}
							onDelete={() => {
								fileToDelete = file.fileName;
							}}
						/>
					</li>
				{/each}
			</ul>
		{/if}
		<KnowledgeUpload onUpload={loadFiles} />
	{/snippet}
</Menu>

<Confirm
	show={fileToDelete !== undefined}
	msg={`Are you sure you want to delete ${fileToDelete}?`}
	onsuccess={deleteFile}
	oncancel={() => (fileToDelete = undefined)}
/>
