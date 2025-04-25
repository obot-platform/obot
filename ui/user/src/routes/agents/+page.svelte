<script lang="ts">
	import Notifications from '$lib/components/Notifications.svelte';
	import type { PageProps } from './$types';
	import { goto } from '$app/navigation';
	import Navbar from '$lib/components/Navbar.svelte';
	import { darkMode, errors } from '$lib/stores';
	import Footer from '$lib/components/Footer.svelte';
	import { formatTime } from '$lib/time';
	import { getProjectImage } from '$lib/image';
	import { ListFilter, Plus, Trash2, X } from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import { fade, fly, slide } from 'svelte/transition';
	import { faker } from '@faker-js/faker';
	import Select from '$lib/components/Select.svelte';
	import McpCatalog from '$lib/components/mcp/McpCatalog.svelte';
	import { ChatService, EditorService, type Project } from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	interface CreateAgent {
		prompt: string;
		mcps: string[];
		systemPrompt: string;
		tasks: string[];
		model: string;
	}

	let { data }: PageProps = $props();
	let createDialog = $state<HTMLDialogElement>();
	let createAgent = $state<CreateAgent>();
	let createAgentStep = $state(0);
	let showMcpCatalog = $state(false);

	let agents = $state<Project[]>(
		data.projects.sort((a, b) => {
			return new Date(b.created).getTime() - new Date(a.created).getTime();
		})
	);
	let mcpsMap = $derived(new Map(data.mcps.map((mcp) => [mcp.id, mcp])));
	let toolsMap = $derived(new Map(data.tools.map((tool) => [tool.id, tool])));
	let selectedMcps = $derived(new Set(createAgent?.mcps ?? []));

	let toDelete = $state<Project>();

	const mockAgents = [
		{
			id: '1',
			label: 'OpenAI GPT-4'
		},
		{
			id: '2',
			label: 'Claude 3.5 Sonnet'
		},
		{
			id: '3',
			label: 'Groq Llama 3.2 70B'
		}
	];

	async function createNew() {
		// TEMPORARY:
		try {
			const project = await EditorService.createObot();
			await ChatService.updateProject({
				...project,
				prompt: createAgent?.prompt ?? ''
			});

			const tools = (await ChatService.listTools(project.assistantID, project.id)).items;
			for (const mcpId of createAgent?.mcps ?? []) {
				const mcp = mcpsMap.get(mcpId);
				if (mcp) {
					await ChatService.configureProjectMCP(project.assistantID, project.id, mcpId);
					const matchingToolIndex = tools.findIndex((tool) => tool.id === mcpId);
					if (matchingToolIndex !== -1) {
						tools[matchingToolIndex].enabled = true;
					}
				}
			}

			await ChatService.updateProjectTools(project.assistantID, project.id, {
				items: tools
			});
			await goto(`/o/${project.id}`);
		} catch (error) {
			errors.append((error as Error).message);
		}
	}
</script>

<div class="flex min-h-dvh flex-col items-center">
	<Navbar />
	<main
		class="bg-surface1 relative flex w-full grow flex-col items-center justify-center dark:bg-black"
	>
		<div class="flex w-full max-w-(--breakpoint-xl) flex-col gap-6 py-12">
			<div class="flex justify-between">
				<h1 class="text-2xl font-semibold">Agents</h1>
				<div class="flex items-center gap-4">
					<button
						class="icon-button dark:bg-surface1 hover:text-blue bg-white shadow-sm shadow-gray-100"
						use:tooltip={'Filter Agents'}
					>
						<ListFilter class="size-6" />
					</button>
					<button
						class="button-primary flex items-center gap-1 text-sm"
						onclick={() => {
							createDialog?.showModal();
							createAgent = {
								prompt: '',
								mcps: ['google-search-bundle', 'google-calendar-bundle', 'firecrawl'],
								systemPrompt: faker.lorem.paragraph(),
								tasks: [faker.lorem.sentence(), faker.lorem.sentence(), faker.lorem.sentence()],
								model: '1'
							};
						}}
					>
						<Plus class="size-6" /> Create New Agent
					</button>
				</div>
			</div>

			<div class="dark:bg-surface2 w-full overflow-hidden bg-white shadow-sm">
				<table class="w-full border-collapse">
					<thead class="dark:bg-surface1 bg-surface2">
						<tr>
							<th class="text-md w-1/2 px-4 py-2 text-left font-medium text-gray-500">Agent</th>
							<th class="text-md w-1/4 px-4 py-2 text-left font-medium text-gray-500">Owner</th>
							<th class="text-md w-1/4 px-4 py-2 text-left font-medium text-gray-500">Created</th>
							<th class="text-md w-auto px-4 py-2 text-left font-medium text-gray-500">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each agents as project (project.id)}
							<tr class="border-surface2 dark:border-surface2 border-t shadow-xs">
								<td>
									<a href={`/o/${project.id}`}>
										<div class="flex h-full w-full items-center gap-2 px-4 py-2">
											<img
												src={getProjectImage(project, darkMode.isDark)}
												class="size-10 rounded-full shadow-md"
												alt={project.name}
											/>
											<div class="flex flex-col">
												<h4
													class="line-clamp-1 text-sm font-medium"
													class:text-gray-500={!project.name}
												>
													{project.name || 'Untitled'}
												</h4>
												<p
													class="line-clamp-1 text-xs font-light"
													class:text-gray-300={!project.description}
												>
													{project.description || 'No description'}
												</p>
											</div>
										</div>
									</a>
								</td>
								<td class="text-sm font-light">
									<a class="flex h-full w-full px-4 py-2" href={`/o/${project.id}`}>Unspecified</a>
								</td>
								<td class="text-sm font-light">
									<a class="flex h-full w-full px-4 py-2" href={`/o/${project.id}`}
										>{formatTime(project.created)}</a
									>
								</td>
								<td class="flex justify-end px-4 py-2 text-sm font-light">
									<button class="icon-button" onclick={() => (toDelete = project)}>
										<Trash2 class="size-4" />
									</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	</main>
	<Footer />
	<Notifications />
</div>

<dialog
	bind:this={createDialog}
	class="h-dvh w-dvw overflow-hidden border-0 bg-transparent shadow-none focus:ring-0 focus:outline-none active:ring-0 active:outline-none"
>
	<div class="flex h-full w-full items-center justify-center gap-2">
		{#if createAgent}
			{#if createAgentStep === 0}
				<div
					class="bg-surface1 default-scrollbar-thin relative flex h-full max-h-dvh w-3xl flex-col gap-4 overflow-y-auto rounded-xl px-8 py-5 dark:bg-black"
					in:fly={{ x: -1000, delay: 200 }}
					out:fly={{ x: -1000, duration: 200 }}
				>
					<button
						onclick={() => {
							createDialog?.close();
							createAgent = undefined;
						}}
						class="absolute top-4 right-4 text-gray-500 transition-colors duration-300 hover:text-black"
					>
						<X class="size-8" />
					</button>

					<h3 class="text-xl font-semibold">What do you want to accomplish?</h3>
					<textarea
						class="dark:bg-surface1 text-md max-h-64 w-full grow resize-none rounded-lg bg-white p-4 shadow-sm"
						rows="3"
						placeholder="I want to build a website that sells products..."
						use:autoHeight
						bind:value={createAgent.prompt}
					></textarea>
					<div class="flex items-center gap-4">
						<button class="button-secondary w-full text-gray-500 hover:text-black"
							>I dunno, just playing around</button
						>
						<button class="button-primary w-full" onclick={() => (createAgentStep = 1)}
							>Continue</button
						>
					</div>

					<div class="border-surface3 flex w-full flex-col gap-2 border-t pt-4">
						<h4 class="text-base font-semibold">Or choose from an Agent Template</h4>
						<div
							class="default-scrollbar-thin flex min-h-1 grow flex-col gap-2 overflow-y-auto p-1"
						>
							{#each data.shares as share}
								<button
									class="hover:bg-surface2 flex w-full gap-4 rounded-lg bg-white p-2 text-left shadow-sm"
								>
									<img
										src={getProjectImage(share, darkMode.isDark)}
										class="size-10 rounded-full"
										alt={share.name}
									/>
									<div class="flex flex-col">
										<h4 class="text-sm font-medium">{share.name}</h4>
										<p class="text-xs font-light">{share.description}</p>
									</div>
								</button>
							{/each}
						</div>
					</div>
				</div>
			{:else if createAgentStep === 1}
				<div
					class="bg-surface1 default-scrollbar-thin relative flex h-full max-h-dvh w-3xl flex-col gap-8 overflow-y-auto rounded-xl px-8 py-5 dark:bg-black"
					class:w-xl={showMcpCatalog}
					in:fly={{ x: 1000, delay: 200 }}
					out:fly={{ x: 1000, duration: 200 }}
				>
					<button
						onclick={() => {
							createDialog?.close();
							createAgent = undefined;
							showMcpCatalog = false;
							createAgentStep = 0;
						}}
						class="absolute top-4 right-4 text-gray-500 transition-colors duration-300 hover:text-black"
					>
						<X class="size-8" />
					</button>
					<div class="flex flex-col gap-2">
						<h3 class="text-xl font-semibold">Review and Launch Agent</h3>
						<p class="border-surface3 text-md mb-4 border-b pb-4 font-light text-gray-500">
							We’ve set up your agent using your prompt as a guide. Feel free to review and adjust
							any settings before moving forward — you’ll be able to make changes after launch as
							well.
						</p>

						<div class="flex items-center justify-between">
							<h4 class="text-base font-semibold">MCP Servers</h4>
							<button
								class="button-small text-sm font-light"
								onclick={() => (showMcpCatalog = true)}
							>
								<Plus class="size-4" /> Modify Selected
							</button>
						</div>
						<div class="bg-surface2 grid grid-cols-2 gap-2 rounded-lg p-2 shadow-inner">
							{#each createAgent.mcps as mcpId, index (mcpId)}
								{@const mcp = mcpsMap.get(mcpId)}
								{#if mcp}
									<div
										class="dark:bg-surface1 border-surface2 flex items-center gap-2 rounded-lg bg-white p-2 shadow-sm"
									>
										<div class="flex grow gap-2">
											<img src={mcp.server.icon} class="size-5" alt={mcp.server.name} />
											<div class="flex flex-col">
												<p class="line-clamp-1 text-sm font-medium">{mcp.server.name}</p>
												<p class="line-clamp-1 text-xs font-light">{mcp.server.description}</p>
											</div>
										</div>
										<button
											class="icon-button"
											onclick={() => {
												if (!createAgent) return;
												createAgent = {
													...createAgent,
													mcps: createAgent.mcps.filter((mcpId) => mcpId !== mcp.id)
												};
											}}
										>
											<X class="size-4" />
										</button>
									</div>
								{/if}
							{/each}
						</div>
					</div>
					<div class="flex flex-col gap-2">
						<h4 class="text-base font-semibold">Select Your Model</h4>
						<Select options={mockAgents} selected={createAgent.model} onSelect={() => {}} />
					</div>

					<div class="flex flex-col gap-2">
						<h4 class="text-base font-semibold">System Prompt</h4>
						<textarea
							class="dark:bg-surface1 text-md max-h-64 w-full grow resize-none rounded-lg bg-white p-4 shadow-sm"
							rows="3"
							placeholder="Describe your agent's personality, goals, and any other important information."
							use:autoHeight
							bind:value={createAgent.systemPrompt}
						></textarea>
					</div>

					<div class="flex flex-col gap-2">
						<div class="flex items-center justify-between">
							<h4 class="text-base font-semibold">Task Descriptions</h4>
						</div>
						<div class="flex flex-col gap-2">
							{#each createAgent.tasks as _task, index}
								<div class="flex items-center gap-2">
									<input
										type="text"
										class="dark:bg-surface1 text-md w-full grow resize-none rounded-lg bg-white px-4 py-2 shadow-sm"
										placeholder="Enter a task"
										bind:value={createAgent.tasks[index]}
									/>
									<button class="icon-button" onclick={() => createAgent?.tasks.splice(index, 1)}>
										<Trash2 class="size-4" />
									</button>
								</div>
							{/each}
						</div>
					</div>

					<div class="flex items-center gap-4">
						<button
							class="button-secondary w-full text-gray-500 hover:text-black"
							onclick={() => {
								createAgentStep = 0;
								showMcpCatalog = false;
							}}
						>
							Go Back
						</button>
						<button class="button-primary w-full" onclick={createNew}>Launch Agent</button>
					</div>
				</div>
				{#if showMcpCatalog}
					<div
						class="relative flex h-full max-h-dvh grow flex-col overflow-hidden rounded-xl bg-white dark:bg-black"
						in:fly={{ x: 1000, duration: 200 }}
						out:fade={{ duration: 0 }}
					>
						<button
							onclick={() => {
								showMcpCatalog = false;
							}}
							class="absolute top-4 right-4 text-gray-500 transition-colors duration-300 hover:text-black"
						>
							<X class="size-8" />
						</button>
						<h2 class="px-12 pt-4 text-3xl font-semibold">MCP Servers</h2>
						<div
							class="h-inherit default-scrollbar-thin flex w-full grow flex-col overflow-y-auto pr-4"
						>
							<McpCatalog
								inline
								mcps={data.mcps}
								hideLogo
								selectedMcpIds={selectedMcps}
								onSubmitMcp={(mcp) => {
									if (!createAgent) return;
									createAgent = {
										...createAgent,
										mcps: [...createAgent.mcps, mcp.id]
									};
								}}
							/>
						</div>
					</div>
				{/if}
			{/if}
		{/if}
	</div>
</dialog>

<Confirm
	msg={`Delete agent: ${toDelete?.name ?? 'Untitled'}?`}
	show={!!toDelete}
	onsuccess={async () => {
		if (!toDelete) return;
		try {
			await ChatService.deleteProject(toDelete.assistantID, toDelete.id);
			agents = agents.filter((p) => p.id !== toDelete?.id);
		} finally {
			toDelete = undefined;
		}
	}}
	oncancel={() => (toDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Agents</title>
</svelte:head>
