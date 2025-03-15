<script lang="ts">
	import { ChatService, type Project } from '$lib/services';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { type KnowledgeFile as KnowledgeFileType } from '$lib/services';
	import { fade } from 'svelte/transition';
	import KnowledgeUpload from '$lib/components/navbar/KnowledgeUpload.svelte';
	import KnowledgeFile from '$lib/components/navbar/KnowledgeFile.svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let knowledgeFiles = $state<KnowledgeFileType[]>([]);

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

{#snippet toolList(files: KnowledgeFileType[])}
	<ul class="flex flex-col gap-2">
		{#each files as file}
			{#key file.fileName}
				<KnowledgeFile {file} onDelete={() => remove(file)} />
			{/key}
		{/each}
	</ul>
{/snippet}

<CollapsePane header="File Knowledge" onOpen={() => reload()}>
	<div class="flex flex-col gap-2">
		<ul class="flex flex-col gap-2">
			{@render toolList(knowledgeFiles)}
		</ul>
		<div class="self-end" in:fade>
			<KnowledgeUpload onUpload={() => reload()} {project} />
		</div>
	</div>
</CollapsePane>
