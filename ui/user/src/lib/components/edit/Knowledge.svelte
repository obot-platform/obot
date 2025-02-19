<script lang="ts">
	import { ChatService, type Project } from '$lib/services';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { knowledgeFiles } from '$lib/stores';
	import { type KnowledgeFile as KnowledgeFileType } from '$lib/services';
	import { fade } from 'svelte/transition';
	import KnowledgeUpload from '$lib/components/navbar/KnowledgeUpload.svelte';
	import KnowledgeFile from '$lib/components/navbar/KnowledgeFile.svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();

	async function reload() {
		knowledgeFiles.items = (await ChatService.listKnowledgeFiles({ projectID: project.id })).items;
	}

	async function remove(tool: KnowledgeFileType) {
		await ChatService.deleteKnowledgeFile(tool.fileName);
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

<CollapsePane header="Knowledge">
	<div class="flex flex-col gap-2">
		<ul class="flex flex-col gap-2">
			{@render toolList(knowledgeFiles.items)}
		</ul>
		<div class="self-end" in:fade>
			<KnowledgeUpload onUpload={() => reload()} />
		</div>
	</div>
</CollapsePane>
