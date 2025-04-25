<script lang="ts">
	import { ChatService, type Assistant, type Project } from '$lib/services';
	import { type KnowledgeFile as KnowledgeFileType } from '$lib/services';
	import KnowledgeFile from '$lib/components/edit/knowledge/KnowledgeFile.svelte';
	import { Plus, Trash2 } from 'lucide-svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import KnowledgeUpload from '$lib/components/edit/knowledge/KnowledgeUpload.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		assistant?: Assistant;
	}

	let { project, currentThreadID = $bindable(), assistant }: Props = $props();
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

	async function loadFiles() {
		if (!currentThreadID) {
			return;
		}
		knowledgeFiles = (
			await ChatService.listKnowledgeFiles(project.assistantID, project.id, {
				threadID: currentThreadID
			})
		).items;
	}

	async function remove(file: KnowledgeFileType) {
		await ChatService.deleteKnowledgeFile(project.assistantID, project.id, file.fileName);
		return reload();
	}
</script>

<CollapsePane classes={{ header: 'pl-3 py-2', content: 'p-2' }} iconSize={5}>
	{#snippet header()}
		<span class="flex grow items-center gap-2 text-start text-sm font-extralight"> Knowledge </span>
	{/snippet}
	<div class="flex flex-col gap-2">
		<p class="py-2 text-xs font-light text-gray-500">
			Add files or websites to your agent's knowledge base.
		</p>

		<p class="text-sm font-medium">Files</p>

		<div class="flex flex-col gap-2 pr-3">
			{#if knowledgeFiles.length > 0}
				<div class="flex flex-col gap-4 text-sm">
					{#each knowledgeFiles as file}
						{#key file.fileName}
							<KnowledgeFile {file} onDelete={() => remove(file)} iconSize={4} />
						{/key}
					{/each}
				</div>
			{/if}
		</div>

		<div class="flex justify-end">
			<KnowledgeUpload
				onUpload={loadFiles}
				{project}
				{currentThreadID}
				classes={{ button: 'w-fit text-xs' }}
			/>
		</div>

		{#if assistant?.websiteKnowledge?.siteTool}
			<p class="text-md font-semibold">Websites</p>

			<div class="flex flex-col gap-4">
				{@render websiteKnowledgeList()}
			</div>
		{/if}
	</div>
</CollapsePane>

{#snippet websiteKnowledgeList()}
	<div class="flex flex-col gap-2">
		{#if project.websiteKnowledge?.sites}
			<table class="w-full text-left">
				<thead class="text-sm">
					<tr>
						<th class="font-light">Website Address</th>
						<th class="font-light">Description</th>
					</tr>
				</thead>
				<tbody>
					{#each project.websiteKnowledge.sites as _, i (i)}
						<tr class="group">
							<td>
								<input
									bind:value={project.websiteKnowledge.sites[i].site}
									placeholder="example.com"
									class="ghost-input border-surface2 w-3/4"
								/>
							</td>
							<td>
								<textarea
									class="ghost-input border-surface2 w-5/6 resize-none"
									bind:value={project.websiteKnowledge.sites[i].description}
									rows="1"
									placeholder="Description"
									use:autoHeight
								></textarea>
							</td>
							<td class="flex justify-end">
								<button
									class="icon-button"
									onclick={() => {
										project.websiteKnowledge?.sites?.splice(i, 1);
									}}
								>
									<Trash2 class="size-5" />
								</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
		<div class="self-end">
			<button
				class="button-small"
				onclick={() => {
					if (!project.websiteKnowledge) {
						project.websiteKnowledge = {
							sites: [{}]
						};
					} else if (!project.websiteKnowledge.sites) {
						project.websiteKnowledge.sites = [{}];
					} else {
						project.websiteKnowledge.sites.push({});
					}
				}}
			>
				<Plus class="size-4" />
				Website
			</button>
		</div>
	</div>
{/snippet}
