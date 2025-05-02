<script lang="ts">
	import { ChatService, type Assistant, type Project } from '$lib/services';
	import { type KnowledgeFile as KnowledgeFileType } from '$lib/services';
	import KnowledgeFile from '$lib/components/edit/knowledge/KnowledgeFile.svelte';
	import { Plus, Trash2 } from 'lucide-svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import KnowledgeUpload from '$lib/components/edit/knowledge/KnowledgeUpload.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';

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

	async function remove(file: KnowledgeFileType) {
		await ChatService.deleteKnowledgeFile(project.assistantID, project.id, file.fileName);
		return reload();
	}
</script>

<CollapsePane
	classes={{ header: 'pl-3 py-2', content: 'p-2' }}
	iconSize={5}
	onOpen={() => reload()}
	header="Knowledge"
	helpText={HELPER_TEXTS.knowledge}
>
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
				onUpload={() => reload()}
				{project}
				{currentThreadID}
				classes={{ button: 'w-fit text-xs' }}
			/>
		</div>

		{#if assistant?.websiteKnowledge?.siteTool}
			<p class="text-sm font-medium">Websites</p>

			<div class="flex flex-col gap-4">
				{@render websiteKnowledgeList()}
			</div>
		{/if}
	</div>
</CollapsePane>

{#snippet websiteKnowledgeList()}
	<div class="flex flex-col gap-2">
		{#if project.websiteKnowledge?.sites}
			<div class="flex flex-col gap-2">
				{#each project.websiteKnowledge.sites as _, i (i)}
					<div class="group flex gap-2 rounded-md bg-white p-2 text-xs shadow-sm">
						<div class="flex grow flex-col gap-2">
							<div>
								<label for={`website-address-${i}`} class="text-xs font-light"
									>Website Address</label
								>
								<input
									id={`website-address-${i}`}
									bind:value={project.websiteKnowledge.sites[i].site}
									placeholder="example.com"
									class="ghost-input border-surface2 w-full"
								/>
							</div>
							<div>
								<label for={`website-description-${i}`} class="text-xs font-light"
									>Description</label
								>
								<textarea
									id={`website-description-${i}`}
									class="ghost-input border-surface2 w-full resize-none"
									bind:value={project.websiteKnowledge.sites[i].description}
									rows="1"
									placeholder="Description"
									use:autoHeight
								></textarea>
							</div>
						</div>
						<div class="flex items-center justify-end">
							<button
								class="icon-button size-fit"
								onclick={() => {
									project.websiteKnowledge?.sites?.splice(i, 1);
								}}
							>
								<Trash2 class="size-4" />
							</button>
						</div>
					</div>
				{/each}
			</div>
		{/if}
		<div class="self-end">
			<button
				class="button-small text-xs"
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
