<script lang="ts">
	import { ChatService, type Assistant, type Project } from '$lib/services';
	import { type KnowledgeFile as KnowledgeFileType } from '$lib/services';
	import KnowledgeFile from '$lib/components/edit/knowledge/KnowledgeFile.svelte';
	import { Plus, Trash2, TriangleAlert } from 'lucide-svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import KnowledgeUpload from '$lib/components/edit/knowledge/KnowledgeUpload.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import { hasTool } from '$lib/tools';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { twMerge } from 'tailwind-merge';
	import InfoTooltip from '../InfoTooltip.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		assistant?: Assistant;
	}

	const projectTools = getProjectTools();
	let { project, currentThreadID = $bindable(), assistant }: Props = $props();
	let knowledgeFiles = $state<KnowledgeFileType[]>([]);
	let hasKnowledgeCapability = $derived(
		!!(hasTool(projectTools.tools, 'knowledge') || assistant?.websiteKnowledge?.siteTool)
	);
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

<div class="flex flex-col gap-2">
	<div
		class={twMerge(
			'flex items-center gap-2',
			!hasKnowledgeCapability && 'text-gray-400 dark:text-gray-600'
		)}
	>
		<h2 class="text-xl font-semibold">Knowledge Files</h2>
		{#if !hasKnowledgeCapability}
			<div use:tooltip={'Capability Required'}>
				<TriangleAlert class="size-6" />
			</div>
		{:else}
			<InfoTooltip text={HELPER_TEXTS.knowledge} class="size-4" classes={{ icon: 'size-4' }} />
		{/if}
	</div>

	<div class="flex flex-col gap-4">
		{#if !hasKnowledgeCapability}
			<p class="flex items-center gap-1 text-sm font-light text-gray-500">
				<span> Enable Knowledge in "Built-In Capabilities" to add knowledge to your project. </span>
			</p>
		{/if}
		{#if hasTool(projectTools.tools, 'knowledge')}
			<div class="flex flex-col gap-2">
				{#if knowledgeFiles.length > 0}
					<div
						class="text-md dark:bg-surface1 dark:border-surface3 gap-4 rounded-md border border-transparent bg-white shadow-sm"
					>
						{#each knowledgeFiles as file (file.fileName)}
							{#key file.fileName}
								<KnowledgeFile
									classes={{ button: 'p-4', delete: 'flex-shrink-0 icon-button mr-2' }}
									{file}
									onDelete={() => remove(file)}
									iconSize={5}
								/>
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
					classes={{ button: 'w-fit text-md' }}
				/>
			</div>
		{/if}

		{#if assistant?.websiteKnowledge?.siteTool}
			<p class="text-sm font-medium">Websites</p>

			<div class="flex flex-col gap-4">
				{@render websiteKnowledgeList()}
			</div>
		{/if}
	</div>
</div>

{#snippet websiteKnowledgeList()}
	<div class="flex flex-col gap-2">
		{#if project.websiteKnowledge?.sites}
			<div class="flex flex-col gap-2">
				{#each project.websiteKnowledge.sites as _, i (i)}
					<div
						class="group dark:border-surface3 flex gap-2 rounded-md bg-white p-2 text-xs shadow-sm dark:border dark:bg-black"
					>
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
