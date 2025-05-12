<script lang="ts">
	import { ChatService, type Project } from '$lib/services';
	import { type KnowledgeFile as KnowledgeFileType } from '$lib/services';
	import KnowledgeFile from '$lib/components/edit/knowledge/KnowledgeFile.svelte';
	import { Plus, Trash2, TriangleAlert } from 'lucide-svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import KnowledgeUpload from '$lib/components/edit/knowledge/KnowledgeUpload.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import { hasTool } from '$lib/tools';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		project: Project;
		currentThreadID?: string;
	}

	const projectTools = getProjectTools();
	let { project, currentThreadID = $bindable() }: Props = $props();
	let knowledgeFiles = $state<KnowledgeFileType[]>([]);
	let hasKnowledgeCapability = $derived(
		!!(hasTool(projectTools.tools, 'knowledge') || project.websiteKnowledge)
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

<CollapsePane
	classes={{
		header: 'pl-3 py-2',
		content: 'p-2'
	}}
	iconSize={5}
	onOpen={() => reload()}
	helpText={HELPER_TEXTS.knowledge}
>
	{#snippet header()}
		<span
			class={twMerge(
				'flex grow items-center justify-between gap-1 text-sm',
				!hasKnowledgeCapability && 'text-gray-400 dark:text-gray-600'
			)}
		>
			Knowledge
			{#if !hasKnowledgeCapability}
				<div use:tooltip={'Capability Required'}>
					<TriangleAlert class="size-4" />
				</div>
			{/if}
		</span>
	{/snippet}
	<div class="flex flex-col gap-2">
		{#if !hasKnowledgeCapability}
			<p class="flex items-center gap-1 text-xs font-light text-gray-500">
				<span>
					Enable the Knowledge or Website Knowledge capability to add knowledge to your agent.
				</span>
			</p>
		{/if}
		{#if hasTool(projectTools.tools, 'knowledge')}
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
		{/if}

		{#if project.websiteKnowledge}
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
					if (!project.websiteKnowledge?.sites) return;
					project.websiteKnowledge.sites.push({});
				}}
			>
				<Plus class="size-4" />
				Website
			</button>
		</div>
	</div>
{/snippet}
