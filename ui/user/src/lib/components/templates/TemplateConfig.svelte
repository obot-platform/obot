<script lang="ts">
	import { fade } from 'svelte/transition';
	import type {
		ProjectTemplate,
		ProjectTemplateManifest,
		ToolReference,
		ProjectMCP,
		File,
		KnowledgeFile
	} from '$lib/services';
	import {
		updateProjectTemplate,
		deleteProjectTemplate,
		listProjectMCPs,
		listFiles,
		EditorService,
		ChatService,
		getProjectTemplate
	} from '$lib/services';
	import { XIcon, FileText, Image, Download, Loader2, Pencil, Star, Globe } from 'lucide-svelte';
	import { closeSidebarConfig } from '$lib/context/layout.svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { IGNORED_BUILTIN_TOOLS } from '$lib/constants';
	import { sortShownToolsPriority } from '$lib/sort';
	import ToolPill from '$lib/components/ToolPill.svelte';
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { onMount, onDestroy } from 'svelte';
	import { isImage } from '$lib/image';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { profile } from '$lib/stores';

	interface Props {
		template: ProjectTemplate;
	}

	let { template = $bindable<ProjectTemplate>() }: Props = $props();
	const layout = getLayout();

	// Check if current user is admin
	let isAdmin = $derived(profile.current.role === 1);

	let loading = $state(!template.ready);
	let toolsMap = $state(new Map<string, ToolReference>());
	let url = $derived(template?.publicID ? `${window.location.origin}/t/${template.publicID}` : '');
	let toDelete = $state(false);
	let mcpServers = $state<ProjectMCP[]>([]);
	let files = $state<File[]>([]);
	let knowledgeFiles = $state<KnowledgeFile[]>([]);
	let loadTimeout: number | undefined;
	let editingName = $state(false);
	let nameInput = $state<HTMLInputElement | null>(null);

	async function loadTemplate() {
		try {
			template = await getProjectTemplate(template.assistantID, template.projectID, template.id);

			if (!template.ready) {
				loading = true;
				loadTimeout = setTimeout(loadTemplate, 1000);
				return;
			}

			clearTimeout(loadTimeout);
			mcpServers = await listProjectMCPs(template.assistantID, template.id);
			const filesResponse = await listFiles(template.assistantID, template.id);
			files = filesResponse.items || [];

			// Load knowledge files
			const knowledgeResponse = await ChatService.listKnowledgeFiles(
				template.assistantID,
				template.id
			);
			knowledgeFiles = knowledgeResponse.items || [];
			loading = false;
		} catch (error) {
			console.error('Failed to load resources:', error);
		}
	}

	onMount(async () => {
		loadTemplate();
	});

	onDestroy(() => {
		if (loadTimeout) {
			clearTimeout(loadTimeout);
		}
	});

	function getTemplateTools(template: ProjectTemplate) {
		if (!template.projectSnapshot.tools || !toolsMap.size) return [];
		return template.projectSnapshot.tools
			.filter((t) => !IGNORED_BUILTIN_TOOLS.has(t))
			.sort(sortShownToolsPriority)
			.map((t) => toolsMap.get(t))
			.filter((t): t is ToolReference => !!t);
	}

	async function updateTemplate() {
		try {
			const updatedManifest: ProjectTemplateManifest = {
				name: template.name,
				public: template.public,
				featured: template.featured
			};

			template = await updateProjectTemplate(
				template.assistantID,
				template.projectID,
				template.id,
				updatedManifest
			);
		} catch (error) {
			console.error('Failed to update template:', error);
		}
	}

	async function handleDeleteTemplate() {
		try {
			await deleteProjectTemplate(template.assistantID, template.id, template.id);
			closeSidebarConfig(layout);
		} catch (error) {
			console.error('Failed to delete template:', error);
		}
	}

	function downloadFile(file: File) {
		EditorService.download([], template, file.name);
	}

	function toggleNameEdit() {
		editingName = !editingName;
		if (editingName) {
			setTimeout(() => nameInput?.focus(), 0);
		}
	}

	function saveTemplateName() {
		if (template.name?.trim()) {
			updateTemplate();
		}
		editingName = false;
	}

	$effect(() => {
		if (editingName) {
			setTimeout(() => nameInput?.focus(), 0);
		}
	});

	const templateTools = $derived(getTemplateTools(template));
</script>

<div class="flex w-full flex-col gap-4 p-5" in:fade>
	<div class="flex w-full items-center justify-end">
		<button
			onclick={() => closeSidebarConfig(layout)}
			class="ml-auto text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
		>
			<XIcon class="size-4" />
		</button>
	</div>

	<div class="flex items-center gap-3">
		<AssistantIcon project={template.projectSnapshot} class="shrink-0" />
		<div class="flex flex-col">
			{#if editingName}
				<div class="flex items-center">
					<input
						bind:this={nameInput}
						bind:value={template.name}
						type="text"
						placeholder="Template Name"
						class="border-b border-gray-300 bg-transparent py-1 text-base font-medium outline-none dark:border-gray-600"
						onkeydown={(e) => {
							if (e.key === 'Enter') saveTemplateName();
							if (e.key === 'Escape') {
								editingName = false;
							}
						}}
						onblur={saveTemplateName}
					/>
				</div>
			{:else}
				<div class="flex items-center gap-2">
					<h3 class="text-base font-medium">
						{template.name || template.projectSnapshot.name || 'Unnamed Template'}
					</h3>
					<button
						class="text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
						onclick={toggleNameEdit}
						use:tooltip={'Edit template name'}
					>
						<Pencil class="size-4" />
					</button>
				</div>
			{/if}
			<div class="flex items-center gap-2 text-[10px] text-gray-500">
				{#if template.featured}
					<div class="flex items-center gap-1" use:tooltip={'Featured template'}>
						<Star class="size-3 text-blue-500" />
						<span>Featured</span>
					</div>
				{/if}
				{#if template.public}
					<div class="flex items-center gap-1" use:tooltip={'Public template'}>
						<Globe class="size-3" />
						<span>Public</span>
					</div>
				{/if}
			</div>
		</div>
	</div>

	<div class="rounded-md border border-gray-100 dark:border-gray-700">
		<div
			class="flex items-center justify-between border-b border-gray-100 p-3 dark:border-gray-700"
		>
			<h3 class="text-sm font-medium">Visibility</h3>
		</div>
		<div class="p-3">
			<div class="flex items-center justify-between py-1.5">
				<div class="flex flex-col">
					<span class="text-sm">Public</span>
					<span class="text-xs text-gray-500"
						>When you're ready to share this template with others, you can make it public.</span
					>
				</div>
				<Toggle
					label="Public Template"
					checked={template?.public ?? false}
					disabled={!isAdmin && template.public && template.featured}
					onChange={() => {
						template.public = !template.public;
						if (!template.public) {
							template.featured = false;
						}
						updateTemplate();
					}}
				/>
			</div>

			{#if isAdmin}
				<div class="flex items-center justify-between py-1.5">
					<div class="flex flex-col">
						<span class="text-sm">Featured</span>
						<span class="text-xs text-gray-500"
							>Make this template appear in the "Featured" category of the Agent Catalog.</span
						>
					</div>
					<Toggle
						label="Featured Template"
						checked={template?.featured ?? false}
						disabled={!isAdmin}
						onChange={() => {
							template.featured = !template.featured;
							if (template.featured) {
								template.public = true;
							}
							updateTemplate();
						}}
					/>
				</div>
			{/if}
		</div>
	</div>

	{#if template.publicID && template.public}
		<div class="rounded-md border border-gray-100 dark:border-gray-700">
			<div class="border-b border-gray-100 p-3 dark:border-gray-700">
				<h3 class="text-sm font-medium">Public URL</h3>
			</div>
			<div class="p-3">
				<div class="flex items-center gap-1">
					<CopyButton text={url} />
					<a href={url} class="overflow-hidden text-sm text-ellipsis hover:underline">{url}</a>
				</div>
			</div>
		</div>
	{/if}

	{#if templateTools.length > 0}
		<div class="rounded-md border border-gray-100 dark:border-gray-700">
			<div class="border-b border-gray-100 p-3 dark:border-gray-700">
				<h3 class="text-sm font-medium">Tools</h3>
			</div>
			<div class="flex flex-wrap gap-2 p-3">
				{#each templateTools as tool}
					<div
						class="flex items-center gap-2 rounded-md bg-gray-50 px-2 py-1 text-xs dark:bg-gray-700"
					>
						<ToolPill {tool} />
						<span>{tool.name}</span>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<div class="rounded-md border border-gray-100 dark:border-gray-700">
		<div class="border-b border-gray-100 p-3 dark:border-gray-700">
			<h3 class="text-sm font-medium">Agent Details</h3>
		</div>
		{#if loading}
			<div class="flex items-center justify-center p-6">
				<div class="flex flex-col items-center gap-2">
					<Loader2 class="size-6 animate-spin text-gray-500" />
					<span class="text-sm text-gray-500">Loading template data...</span>
				</div>
			</div>
		{:else}
			<div class="flex flex-col divide-y divide-gray-100 dark:divide-gray-700">
				{#if template.created}
					<div class="p-3">
						<h4 class="mb-1 text-xs font-medium text-gray-500">Created</h4>
						<p class="text-sm text-gray-600 dark:text-gray-300">
							{new Date(template.created).toLocaleString(undefined, {
								year: 'numeric',
								month: 'short',
								day: 'numeric',
								hour: '2-digit',
								minute: '2-digit',
								second: '2-digit'
							})}
						</p>
					</div>
				{/if}

				{#if template.projectSnapshot.description}
					<div class="p-3">
						<h4 class="mb-1 text-xs font-medium text-gray-500">Description</h4>
						<p class="text-sm text-gray-600 dark:text-gray-300">
							{template.projectSnapshot.description}
						</p>
					</div>
				{/if}

				{#if template.projectSnapshot.prompt}
					<div class="p-3">
						<h4 class="mb-1 text-xs font-medium text-gray-500">System Prompt</h4>
						<p class="text-xs whitespace-pre-wrap text-gray-600 dark:text-gray-300">
							{template.projectSnapshot.prompt}
						</p>
					</div>
				{/if}

				{#if template.projectSnapshot.introductionMessage}
					<div class="p-3">
						<h4 class="mb-1 text-xs font-medium text-gray-500">Introduction Message</h4>
						<p class="text-xs whitespace-pre-wrap text-gray-600 dark:text-gray-300">
							{template.projectSnapshot.introductionMessage}
						</p>
					</div>
				{/if}

				{#if template.projectSnapshot.starterMessages && template.projectSnapshot.starterMessages.length > 0}
					<div class="p-3">
						<h4 class="mb-2 text-xs font-medium text-gray-500">Conversation Starters</h4>
						<div class="flex flex-col gap-2">
							{#each template.projectSnapshot.starterMessages as message}
								<div
									class="w-fit max-w-[90%] rounded-lg rounded-tl-none bg-blue-50 p-2 text-xs whitespace-pre-wrap text-gray-700 dark:bg-gray-700 dark:text-gray-300"
								>
									{message}
								</div>
							{/each}
						</div>
					</div>
				{/if}

				{#if mcpServers.length > 0}
					<div class="p-3">
						<h4 class="mb-2 text-xs font-medium text-gray-500">MCP Servers</h4>
						<div class="flex flex-col gap-2">
							{#each mcpServers as mcp}
								<div
									class="flex w-fit items-center gap-1.5 rounded-md bg-gray-50 px-2 py-1 dark:bg-gray-800"
								>
									<div class="flex-shrink-0 rounded-md bg-white p-1 dark:bg-gray-700">
										<img src={mcp.icon} class="size-3.5" alt={mcp.name} />
									</div>
									<span class="truncate text-xs">{mcp.name}</span>
								</div>
							{/each}
						</div>
					</div>
				{/if}

				{#if files && files.length > 0}
					<div class="p-3">
						<h4 class="mb-2 text-xs font-medium text-gray-500">Starter Files</h4>
						<ul class="flex flex-col gap-1.5">
							{#each files as file}
								<li class="group">
									<div class="flex items-center rounded-md hover:bg-gray-50 dark:hover:bg-gray-800">
										<div class="flex flex-1 items-center gap-1.5 truncate p-1.5 text-start text-sm">
											{#if isImage(file.name)}
												<Image class="size-4 min-w-fit text-gray-500" />
											{:else}
												<FileText class="size-4 min-w-fit text-gray-500" />
											{/if}
											<span class="truncate">{file.name}</span>
										</div>
										<button
											class="icon-button-small ms-2 opacity-0 transition-all duration-200 group-hover:opacity-100"
											onclick={() => downloadFile(file)}
											use:tooltip={'Download file'}
										>
											<Download class="size-4 text-gray-500" />
										</button>
									</div>
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				{#if knowledgeFiles.length > 0}
					<div class="p-3">
						<h4 class="mb-1 text-xs font-medium text-gray-500">Knowledge Files</h4>
						<ul class="mt-2">
							{#each knowledgeFiles as file}
								<li class="mb-1 text-xs text-gray-600 last:mb-0 dark:text-gray-300">
									{file.fileName}
									{#if file.state && file.state !== 'ready'}
										<span class="ml-1 text-[10px] text-gray-500">({file.state})</span>
									{/if}
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				{#if template.projectSnapshot.websiteKnowledge && Object.keys(template.projectSnapshot.websiteKnowledge).length > 0}
					<div class="p-3">
						<h4 class="mb-1 text-xs font-medium text-gray-500">Website Knowledge</h4>
						{#each Object.entries(template.projectSnapshot.websiteKnowledge) as key}
							<div class="mb-1 flex items-start gap-1 last:mb-0">
								<span class="text-xs font-medium text-gray-500">Site:</span>
								<span class="text-xs text-gray-600 dark:text-gray-300">{key}</span>
							</div>
						{/each}
					</div>
				{/if}

				{#if template.projectSnapshot.sharedTasks && template.projectSnapshot.sharedTasks.length > 0}
					<div class="p-3">
						<h4 class="mb-1 text-xs font-medium text-gray-500">Shared Tasks</h4>
						{#each template.projectSnapshot.sharedTasks as task}
							<div class="mb-1 text-xs text-gray-600 last:mb-0 dark:text-gray-300">{task}</div>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>
</div>

<Confirm
	msg={`Are you sure you want to delete this template: ${template.name || 'Unnamed Template'}?`}
	show={toDelete}
	onsuccess={handleDeleteTemplate}
	oncancel={() => (toDelete = false)}
/>
