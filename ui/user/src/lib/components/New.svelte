<script lang="ts">
	import { ChatService, type ProjectTemplate } from '$lib/services';
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import { RotateCcw, X } from 'lucide-svelte';
	import generateName from '$lib/generatename';
	import { assistants } from '$lib/stores';

	interface Props {
		closable?: boolean;
	}

	let dialog: HTMLDialogElement;
	let templates = $state<ProjectTemplate[]>([]);
	let selected = $state<ProjectTemplate>();
	let newName = $state(generateName());
	let { closable = true }: Props = $props();

	async function createProject(template?: ProjectTemplate) {
		if (template) {
			const project = await ChatService.createProjectFromTemplate(template, {
				name: newName
			});
			const assistant = assistants.items.find((a) => a.id === project.assistantID);
			window.location.href = `/${assistant?.alias || project.assistantID}/p/${project.id}`;
		}
		dialog.close();
	}

	export async function show(templateID?: string) {
		templates = (await ChatService.listProjectTemplates()).items;
		selected = undefined;
		if (templateID) {
			selected = templates.find((t) => t.id === templateID);
			if (!selected) {
				selected = await ChatService.getPublicTemplate(templateID);
			}
		}

		dialog.showModal();
	}
</script>

{#snippet header(template: ProjectTemplate, bold?: boolean)}
	{#if bold}
		<h3 class="text-2xl font-semibold">{template.name || 'Untitled'}</h3>
	{:else}
		<div class="flex items-center gap-2">
			<AssistantIcon {template} />
			<h3>{template.name || 'Untitled'}</h3>
		</div>
	{/if}
	<span>{template.description || 'No description'}</span>
{/snippet}

<dialog
	bind:this={dialog}
	class="min-h-1/2 lg:max-h-3/4 colors-surface1 w-full max-w-4xl rounded-3xl"
>
	<div class="relative flex size-full flex-col gap-5 p-5">
		<div class="absolute right-0 top-0 p-2">
			<button
				class="icon-button"
				onclick={() => {
					if (closable) {
						dialog.close();
					} else {
						window.location.href = '/';
					}
				}}
			>
				<X />
			</button>
		</div>
		<div class="flex items-center gap-8">
			{#if selected}
				<AssistantIcon template={selected} class="h-16 w-16" />
			{:else}
				<img src="/user/images/obot-icon-blue.svg" class="h-16 w-16" alt="Obot icon" />
			{/if}
			<h1 class="text-3xl font-semibold">Create your new Obot</h1>
		</div>
		{#if selected}
			<div class="mt-5 flex flex-col gap-5 rounded-3xl">
				<div class="mb-5 grid grid-cols-2">
					<div class="flex flex-col gap-2">
						{@render header(selected, true)}
					</div>
					<div class="flex flex-col gap-2">
						<span class="text-lg">Give your Obot a name</span>
						<div class="colors-background flex rounded-lg p-5">
							<input
								type="text"
								class="grow p-1 text-lg"
								placeholder={selected.name}
								bind:value={newName}
							/>
							<button class="icon-button" onclick={() => (newName = generateName())}>
								<RotateCcw class="h-5 w-5" />
							</button>
						</div>
					</div>
				</div>
				<div class="flex gap-2 self-end">
					<button class="button-secondary" onclick={() => (selected = undefined)}>
						Choose another
					</button>
					<button class="button-primary self-end" onclick={() => createProject(selected)}>
						Create
					</button>
				</div>
			</div>
		{:else}
			<div>
				<p class="p-5">
					An Obot is an AI agent you can chat with or command to do tasks using tools and knowledge.
				</p>
			</div>
			<div class="grid grow gap-3 sm:grid-cols-2 md:grid-cols-3">
				{#each templates as template}
					<button
						class="button flex flex-col items-start gap-5"
						onclick={() => (selected = template)}
					>
						{@render header(template)}
					</button>
				{/each}
			</div>
		{/if}
	</div>
</dialog>
