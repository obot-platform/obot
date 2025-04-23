<script lang="ts">
	import { autoHeight } from '$lib/actions/textarea';
	import { closeSidebarConfig, getLayout, type Layout } from '$lib/context/layout.svelte';
	import type { Project } from '$lib/services';
	import { ChevronsLeft, Plus, Trash2, X } from 'lucide-svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';
	import EditIcon from './edit/EditIcon.svelte';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
	const layout = getLayout();

	const agentViews = ['system-prompt', 'introduction', 'template'];
	const interfaceViews = ['chatbot', 'slack', 'discord', 'sms', 'email', 'webhook', 'interfaces'];

	const isAgentConfigView = $derived(
		layout.sidebarConfig && agentViews.includes(layout.sidebarConfig)
	);
	const isInterfaceConfigView = $derived(
		layout.sidebarConfig && interfaceViews.includes(layout.sidebarConfig)
	);
</script>

<div class="flex w-full" in:fade>
	{#if isAgentConfigView}
		{@const agentTabs = [
			{ label: 'System Prompt', value: 'system-prompt' },
			{ label: 'Introduction & Starter Messages', value: 'introduction' },
			{ label: 'Create Agent Template', value: 'template' }
		]}
		{@render tabs(agentTabs)}
		<div class="w-3xl overflow-visible p-8">
			{#if layout.sidebarConfig === 'system-prompt'}
				<h4 class="mb-8 text-lg font-semibold">System Prompt</h4>
				<div class="flex flex-col gap-4">
					<div class="text-md flex flex-col">
						<p class="text-md mb-4 font-light text-gray-500">
							Describe your agent's personality, goals, and any other relevant information.
						</p>

						<textarea
							id="project-instructions"
							class="dark:bg-surface1 grow resize-none rounded-lg bg-white p-4 shadow-sm"
							rows="3"
							use:autoHeight
							bind:value={project.prompt}
						></textarea>
					</div>
				</div>
			{:else if layout.sidebarConfig === 'introduction'}
				<h4 class="mb-8 text-lg font-semibold">Introduction & Starter Messages</h4>
				<div class="text-md flex w-full gap-4">
					<EditIcon bind:project inline />
					<div class="flex grow flex-col gap-4 pt-5">
						<div class="flex w-full flex-col gap-1">
							<label for="project-name" class="font-semibold">Name</label>
							<input
								id="project-name"
								disabled={!project.editor}
								type="text"
								class="dark:bg-surface1 grow rounded-lg bg-white p-2 shadow-sm"
								bind:value={project.name}
							/>
						</div>
						<div class="flex w-full flex-col gap-1">
							<label for="project-desc" class="font-semibold">Description</label>
							<textarea
								id="project-desc"
								class="dark:bg-surface1 grow resize-none rounded-lg bg-white p-2 shadow-sm"
								disabled={!project.editor}
								rows="1"
								placeholder="A small blurb or tagline summarizing your agent"
								use:autoHeight
								bind:value={project.description}
							></textarea>
						</div>
						<div class="flex w-full flex-col gap-1">
							<label for="project-introduction" class="font-semibold">Introduction</label>
							<textarea
								id="project-introduction"
								class="dark:bg-surface1 grow resize-none rounded-lg bg-white p-2 shadow-sm"
								rows="5"
								placeholder="This will be your agent's go-to message."
								use:autoHeight
								bind:value={project.introductionMessage}
							></textarea>
						</div>
					</div>
				</div>
				<div class="border-surface-3 mt-8 flex flex-col gap-2 border-t pt-6">
					<h4 class="text-lg font-semibold">Starter Messages</h4>
					<p class="text-sm font-light text-gray-500">
						These messages are conversation options that are provided to the user. <br />
						Help break the ice with your agent by providing a few different options!
					</p>
					<div
						class="default-scrollbar-thin mt-2 flex max-h-36 w-full flex-col gap-2 overflow-y-auto p-1 pr-4"
					>
						{#each project.starterMessages?.keys() ?? [] as i}
							{#if project.starterMessages}
								<div class="flex gap-2">
									<textarea
										id="project-instructions"
										class="dark:bg-surface1 grow resize-none rounded-lg bg-white p-2 shadow-sm"
										rows="1"
										use:autoHeight
										bind:value={project.starterMessages[i]}
									></textarea>
									<button
										class="icon-button"
										onclick={() =>
											(project.starterMessages = [
												...(project.starterMessages ?? []).slice(0, i),
												...(project.starterMessages ?? []).slice(i + 1)
											])}
									>
										<Trash2 class="size-4" />
									</button>
								</div>
							{/if}
						{/each}
					</div>
					<div class="flex justify-end">
						<button
							class="button flex items-center gap-1"
							onclick={() => (project.starterMessages = [...(project.starterMessages ?? []), ''])}
						>
							<Plus class="size-4" />
							<span class="text-sm">Starter Message</span>
						</button>
					</div>
				</div>
			{:else}
				{@render underConstruction()}
			{/if}
		</div>
	{:else if isInterfaceConfigView}
		{@const interfacesTabs = [
			{ label: 'Chatbot', value: 'chatbot' },
			{ label: 'Slack', value: 'slack' },
			{ label: 'Discord', value: 'discord' },
			{ label: 'SMS', value: 'sms' },
			{ label: 'Email', value: 'email' },
			{ label: 'Webhook', value: 'webhook' }
		]}
		{@render tabs(interfacesTabs)}
		<div class="p-8">
			{#if layout.sidebarConfig === 'slack'}
				<div></div>
			{:else if layout.sidebarConfig === 'chatbot'}
				<div></div>
			{:else}
				{@render underConstruction()}
			{/if}
		</div>
	{:else if layout.sidebarConfig === 'members'}
		<div></div>
	{/if}
</div>

{#snippet tabs(tabs: { label: string; value: string }[])}
	<div class="text-md border-surface2 mb-8 flex w-xs flex-col border-r-1 px-8 font-light">
		<button
			onclick={() => closeSidebarConfig(layout)}
			class="mb-4 flex w-full items-center gap-1 py-4"
		>
			<ChevronsLeft class="size-6" /> Go Back
		</button>
		{#each tabs as tab}
			<button
				class={twMerge(
					'border-l-4 border-transparent px-4 py-2 text-left',
					layout.sidebarConfig === tab.value && 'border-blue-500'
				)}
				onclick={() => (layout.sidebarConfig = tab.value as Layout['sidebarConfig'])}
			>
				{tab.label}
			</button>
		{/each}
	</div>
{/snippet}

{#snippet underConstruction()}
	<div class="flex w-full font-light">Under Construction</div>
{/snippet}
