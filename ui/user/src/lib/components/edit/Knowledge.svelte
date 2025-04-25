<script lang="ts">
	import { ChatService, type Project } from '$lib/services';
	import { type KnowledgeFile as KnowledgeFileType } from '$lib/services';
	import KnowledgeUpload from '$lib/components/navbar/KnowledgeUpload.svelte';
	import KnowledgeFile from '$lib/components/navbar/KnowledgeFile.svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let knowledgeFiles = $state<KnowledgeFileType[]>([]);

	$effect(() => {
		if (project) {
			reload();
		}
	});

	async function reload() {
		knowledgeFiles = (await ChatService.listKnowledgeFiles(project.assistantID, project.id)).items;
		const pending = knowledgeFiles.find(
			(file) => file.state === 'pending' || file.state === 'ingesting'
		);
		if (pending) {
			setTimeout(reload, 2000);
		}
	}

	async function remove(file: KnowledgeFileType) {
		await ChatService.deleteKnowledgeFile(project.assistantID, project.id, file.fileName);
		return reload();
	}
</script>

{#snippet knowledgeFileList(files: KnowledgeFileType[])}
	<ul class="flex flex-col gap-2 pr-2.5">
		{#each files as file}
			{#key file.fileName}
				<KnowledgeFile {file} onDelete={() => remove(file)} />
			{/key}
		{/each}
	</ul>
{/snippet}

<div class="flex flex-col gap-2" id="sidebar-knowledge">
	<div class="mb-1 flex items-center justify-between">
		<p class="grow text-sm font-semibold">File Knowledge</p>
		<KnowledgeUpload onUpload={() => reload()} {project} compact />
	</div>
	<div class="flex flex-col gap-4">
		{@render knowledgeFileList(knowledgeFiles)}
	</div>
</div>
