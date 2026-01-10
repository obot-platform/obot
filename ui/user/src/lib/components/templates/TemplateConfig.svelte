<script lang="ts">
	import { fade } from 'svelte/transition';
	import type {
		ProjectTemplate,
		ToolReference,
		ProjectMCP,
		KnowledgeFile,
		Task
	} from '$lib/services';
	import {
		deleteProjectTemplate,
		listProjectMCPs,
		ChatService,
		getProjectTemplateForProject,
		createProjectTemplate
	} from '$lib/services';
	import { XIcon, Loader2, Trash2, Server } from 'lucide-svelte';
	import { closeSidebarConfig, getLayout } from '$lib/context/chatLayout.svelte';
	import { DEFAULT_CUSTOM_SERVER_NAME, IGNORED_BUILTIN_TOOLS } from '$lib/constants';
	import { sortShownToolsPriority } from '$lib/sort';
	import ToolPill from '$lib/components/ToolPill.svelte';
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { onMount } from 'svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { poll } from '$lib/utils';
	import { resolve } from '$app/paths';

	interface Props {
		assistantID: string;
		projectID: string;
	}

	let { assistantID, projectID }: Props = $props();

	let template = $state<ProjectTemplate | undefined>();
	const layout = getLayout();

	let loading = $state(true);
	let toolsMap = $state(new Map<string, ToolReference>());
	let url = $derived(template?.publicID ? `${window.location.origin}/t/${template.publicID}` : '');
	let toDelete = $state(false);
	let mcpServers = $state<ProjectMCP[]>([]);
	let knowledgeFiles = $state<KnowledgeFile[]>([]);
	let tasks = $state<Task[]>([]);
	let sourceProjectMcpServers = $state<ProjectMCP[]>([]);
	let hasCompositeServer = $derived(
		sourceProjectMcpServers.some((mcp) => mcp.runtime === 'composite')
	);

	async function loadTemplate() {
		loading = true;
		try {
			// Always fetch the latest template status for this project
			const fetchedTemplate = await getProjectTemplateForProject(assistantID, projectID);
			if (!fetchedTemplate) {
				loading = false;
				return;
			}
			template = fetchedTemplate;

			// If template exists but isn't ready, keep polling
			if (!template.ready || template.projectSnapshotUpgradeInProgress) {
				await poll(
					async () => {
						const next = await getProjectTemplateForProject(assistantID, projectID);
						if (!next) return false;
						template = next;
						return Boolean(template.ready && !template.projectSnapshotUpgradeInProgress);
					},
					{ interval: 500, maxTimeout: 30000 }
				);
			}

			// Convert template thread ID to project ID format (t1xxx -> p1xxx)
			const templateProjectID = template.id.replace('t1', 'p1');
			mcpServers = await listProjectMCPs(template.assistantID, templateProjectID);

			// Load tasks for the template project
			tasks = (await ChatService.listTasks(template.assistantID, templateProjectID)).items;

			// Load knowledge files
			const knowledgeResponse = await ChatService.listKnowledgeFiles(
				template.assistantID,
				templateProjectID
			);
			knowledgeFiles = knowledgeResponse.items || [];
			loading = false;
		} catch (error) {
			if (error instanceof Error && !error.message.includes('404')) {
				console.error('Failed to load resources:', error);
			}
			loading = false;
		}
	}

	onMount(async () => {
		// Load source project's MCP servers to check for composite
		try {
			sourceProjectMcpServers = await listProjectMCPs(assistantID, projectID);
		} catch (error) {
			console.error('Failed to load source project MCP servers:', error);
			sourceProjectMcpServers = [];
		}

		loadTemplate();
	});

	async function createFromSnapshot() {
		if (!(assistantID && projectID)) return;
		const newTpl = await createProjectTemplate(assistantID, projectID);
		template = newTpl;
		await loadTemplate();
	}

	function getTemplateTools(template: ProjectTemplate) {
		if (!template.projectSnapshot.tools || !toolsMap.size) return [];
		return template.projectSnapshot.tools
			.filter((t) => !IGNORED_BUILTIN_TOOLS.has(t))
			.sort(sortShownToolsPriority)
			.map((t) => toolsMap.get(t))
			.filter((t): t is ToolReference => !!t);
	}

	async function handleDeleteTemplate() {
		try {
			await deleteProjectTemplate(assistantID, projectID);
			template = undefined; // Clear the template state
			closeSidebarConfig(layout);
		} catch (error) {
			console.error('Failed to delete template:', error);
		}
	}

	function formatDate(date?: string) {
		if (!date) return '';
		return new Date(date).toLocaleString(undefined, {
			year: 'numeric',
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit',
			second: '2-digit'
		});
	}

	const templateTools = $derived(template ? getTemplateTools(template) : []);
</script>

{#snippet displayTasks(taskList: Task[])}
	{#if taskList.length > 0}
		<div class="p-3">
			<h4 class="text-on-surface1 mb-1 text-xs font-medium">Tasks</h4>
			<div class="flex flex-col gap-2">
				{#each taskList as t (t.id)}
					<div class="rounded-md border border-gray-100 p-2 text-xs dark:border-gray-700">
						<div class="font-medium text-gray-700 dark:text-gray-200">{t.name || t.id}</div>
						{#if t.description}
							<div class="text-on-surface1 mt-0.5">{t.description}</div>
						{/if}
						{#if t.steps && t.steps.length > 0}
							<div class="text-on-surface1 mt-1">
								{t.steps.length} step{t.steps.length === 1 ? '' : 's'}
							</div>
							<ol class="text-on-surface1 mt-1 list-decimal pl-4 text-[11px]">
								{#each t.steps as s, idx (s.id)}
									<li>
										<span>{s.step || 'Step ' + (idx + 1)}</span>
										{#if s.loop && s.loop.length > 0}
											<ul class="mt-0.5 list-[circle] pl-4">
												{#each s.loop as sub (sub)}
													<li class="text-[11px]">{sub}</li>
												{/each}
											</ul>
										{/if}
									</li>
								{/each}
							</ol>
						{/if}
					</div>
				{/each}
			</div>
		</div>
	{/if}
{/snippet}

{#if loading}
	<div class="flex items-center justify-center p-6">
		<div class="flex flex-col items-center gap-2">
			<Loader2 class="text-on-surface1 size-6 animate-spin" />
			<span class="text-on-surface1 text-sm">Loading Project Share...</span>
		</div>
	</div>
{:else}
	<div class="flex w-full flex-col gap-4 p-5" in:fade>
		<div class="flex w-full items-center justify-end">
			<button
				onclick={() => closeSidebarConfig(layout)}
				class="ml-auto text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
			>
				<XIcon class="size-6" />
			</button>
		</div>

		{#if !template}
			<div class="card gap-4">
				<img src="/user/images/share-project-snapshot.webp" class="max-h-48" alt="invitation" />
				<h4 class="text-2xl font-semibold">Project Sharing</h4>

				<div class="flex flex-col items-center gap-6">
					<div class="max-w-2xl space-y-3 text-sm font-light text-gray-600 dark:text-gray-300">
						<p>
							When you share this project, we'll take a snapshot of its configuration that includes
							instructions, MCP servers, knowledge files, and task definitions. You can share the
							generated link with others and they can use it to launch their own instance of the
							project from your snapshot.
						</p>
						<p>
							If you make changes to your project, you can return to this page to take a new
							snapshot. When you do, owners of existing projects launched using your link will be
							notified that an update is available and new instances will automatically get the new
							version.
						</p>
					</div>
					<button
						class="button-primary"
						onclick={createFromSnapshot}
						disabled={hasCompositeServer}
						use:tooltip={hasCompositeServer
							? {
									text: 'Projects with composite MCP servers cannot be shared. Remove composite servers to enable sharing.',
									classes: ['w-md']
								}
							: undefined}
					>
						Share This Project
					</button>
				</div>
			</div>
		{:else}
			<div class="flex items-center gap-3">
				<AssistantIcon project={template.projectSnapshot} class="shrink-0" />
				<div class="flex flex-1 items-center justify-between">
					<h3 class="text-base font-medium">
						{template.projectSnapshot.name || 'Unnamed Template'}
					</h3>
					<div class="flex items-center gap-2">
						{#if template.projectSnapshotStale}
							{#if template.projectSnapshotUpgradeInProgress}
								<div class="text-on-surface1 flex items-center gap-1 text-xs">
									<Loader2 class="size-4 animate-spin" />
									Updating...
								</div>
							{:else}
								<button
									class="button-primary px-3 py-1 text-sm"
									onclick={createFromSnapshot}
									disabled={hasCompositeServer}
									use:tooltip={hasCompositeServer
										? {
												text: 'Projects with composite MCP servers cannot be shared. Remove composite servers to enable sharing.',
												classes: ['w-md']
											}
										: 'Update Project Share with current project state'}
								>
									Update Project Share
								</button>
							{/if}
						{/if}
						<button
							class="icon-button hover:text-red-500"
							onclick={() => (toDelete = true)}
							use:tooltip={'Delete Project Share'}
						>
							<Trash2 class="size-4" />
						</button>
					</div>
				</div>
			</div>

			{#if template.publicID}
				<div class="rounded-md border border-gray-100 dark:border-gray-700">
					<div class="border-b border-gray-100 p-3 dark:border-gray-700">
						<h3 class="text-sm font-medium">Project Share URL</h3>
					</div>
					<div class="p-3">
						<div class="flex items-center gap-1">
							<CopyButton text={url} />
							<a
								href={resolve(url as `/${string}`)}
								class="overflow-hidden text-sm text-ellipsis hover:underline">{url}</a
							>
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
						{#each templateTools as tool (tool.id)}
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
					<h3 class="text-sm font-medium">Project Share details</h3>
				</div>
				<div class="flex flex-col divide-y divide-gray-100 dark:divide-gray-700">
					{#if template.projectSnapshotLastUpgraded}
						<div class="p-3">
							<h4 class="text-on-surface1 mb-1 text-xs font-medium">Last Updated</h4>
							<p class="text-sm text-gray-600 dark:text-gray-300">
								{formatDate(template.projectSnapshotLastUpgraded)}
							</p>
						</div>
					{/if}

					{#if template.projectSnapshot.description}
						<div class="p-3">
							<h4 class="text-on-surface1 mb-1 text-xs font-medium">Description</h4>
							<p class="text-sm text-gray-600 dark:text-gray-300">
								{template.projectSnapshot.description}
							</p>
						</div>
					{/if}

					{#if template.projectSnapshot.prompt}
						<div class="p-3">
							<h4 class="text-on-surface1 mb-1 text-xs font-medium">System Prompt</h4>
							<p class="text-xs whitespace-pre-wrap text-gray-600 dark:text-gray-300">
								{template.projectSnapshot.prompt}
							</p>
						</div>
					{/if}

					{#if template.projectSnapshot.introductionMessage}
						<div class="p-3">
							<h4 class="text-on-surface1 mb-1 text-xs font-medium">Introduction Message</h4>
							<p class="text-xs whitespace-pre-wrap text-gray-600 dark:text-gray-300">
								{template.projectSnapshot.introductionMessage}
							</p>
						</div>
					{/if}

					{#if template.projectSnapshot.starterMessages && template.projectSnapshot.starterMessages.length > 0}
						<div class="p-3">
							<h4 class="text-on-surface1 mb-2 text-xs font-medium">Conversation Starters</h4>
							<div class="flex flex-col gap-2">
								{#each template.projectSnapshot.starterMessages as message (message)}
									<div
										class="bg-primary/10 w-fit max-w-[90%] rounded-lg rounded-tl-none p-2 text-xs whitespace-pre-wrap text-gray-700 dark:bg-gray-700 dark:text-gray-300"
									>
										{message}
									</div>
								{/each}
							</div>
						</div>
					{/if}

					{#if mcpServers.length > 0}
						<div class="p-3">
							<h4 class="text-on-surface1 mb-2 text-xs font-medium">MCP Servers</h4>
							<div class="flex flex-col gap-2">
								{#each mcpServers as mcpServer (mcpServer.id)}
									<div
										class="group hover:bg-surface3 flex w-full items-center rounded-md transition-colors duration-200"
									>
										<div class="rounded-md bg-gray-50 p-1 dark:bg-gray-600">
											{#if mcpServer.icon}
												<img
													src={mcpServer.icon}
													class="size-4"
													alt={mcpServer.alias || mcpServer.name}
												/>
											{:else}
												<Server class="size-4" />
											{/if}
										</div>
										<p
											class="flex w-[calc(100%-24px)] items-center truncate pl-1.5 text-left text-xs font-light"
										>
											{mcpServer.alias || mcpServer.name || DEFAULT_CUSTOM_SERVER_NAME}
										</p>
									</div>
								{/each}
							</div>
						</div>
					{/if}

					{#if knowledgeFiles.length > 0}
						<div class="p-3">
							<h4 class="text-on-surface1 mb-1 text-xs font-medium">Knowledge Files</h4>
							<ul class="mt-2">
								{#each knowledgeFiles as file (file.fileName)}
									<li class="mb-1 text-xs text-gray-600 last:mb-0 dark:text-gray-300">
										{file.fileName}
									</li>
								{/each}
							</ul>
						</div>
					{/if}

					{@render displayTasks(tasks)}
				</div>
			</div>
		{/if}

		{#if template}
			<Confirm
				msg={`Are you sure you want to delete this Project Share: ${template.projectSnapshot.name || 'Unnamed Project Snapshot'}?`}
				show={toDelete}
				onsuccess={handleDeleteTemplate}
				oncancel={() => (toDelete = false)}
			/>
		{/if}
	</div>
{/if}
