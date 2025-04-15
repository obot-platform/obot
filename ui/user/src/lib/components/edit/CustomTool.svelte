<script lang="ts" module>
	const defaultParams = {
		city: 'The city to get the weather for'
	};
	const defaultInstructions: Record<string, string> = {
		javascript: `// Get the city from the environment variable
const city = process.env.CITY || 'Unknown City';

// Generate a random temperature in Fahrenheit
const randomTemperatureFahrenheit = (Math.random() * (104 - 14) + 14).toFixed(2); // Random temperature between 14 and 104 degrees Fahrenheit

// Print the result in Fahrenheit
console.log(\`The current temperature in \${city} is \${randomTemperatureFahrenheit}°F.\`);
	`,
		python: `import os
import random

# Get the city from the environment variable
city = os.getenv('CITY', 'Unknown City')

# Generate a random temperature in Fahrenheit
random_temperature_fahrenheit = random.uniform(14, 104)  # Random temperature between 14 and 104 degrees Fahrenheit

# Print the result in Fahrenheit
print(f'The current temperature in {city} is {random_temperature_fahrenheit:.2f}°F.')
		`,
		script: `#!/bin/bash

# Get the city from the environment variable
CITY=\${CITY:-'Unknown City'}

# Generate a random temperature in Fahrenheit
RANDOM_TEMPERATURE_FAHRENHEIT=$(awk -v min=14 -v max=104 'BEGIN{srand(); print min+rand()*(max-min)}')

# Print the result in Fahrenheit
printf "The current temperature in %s is %.2f°F.\\n" "$CITY" "$RANDOM_TEMPERATURE_FAHRENHEIT"
		`,
		container: ''
	};
</script>

<script lang="ts">
	import { autoHeight } from '$lib/actions/textarea.js';
	import { Container, X, ChevronRight, Trash2, SquarePen } from 'lucide-svelte';
	import { type AssistantTool, ChatService, type Project } from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import Env from '$lib/components/edit/customtool/Env.svelte';
	import Params from '$lib/components/edit/customtool/Params.svelte';
	import Codemirror from '$lib/components/editor/Codemirror.svelte';
	import type { EditorItem } from '$lib/services/editor/index.svelte.js';
	import { onDestroy, onMount } from 'svelte';
	import { newSaveMonitor, type SaveMonitor } from '$lib/save';
	import { responsive } from '$lib/stores';
	import { fade } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';

	interface Props {
		tool: AssistantTool;
		project: Project;
		onSave: (tool: AssistantTool) => Promise<void>;
		onDelete: (tool: AssistantTool) => Promise<void>;
		onClose?: () => void;
	}

	interface SaveState {
		tool: AssistantTool;
		env: Record<string, string>;
	}

	let { tool = $bindable(), project, onSave, onDelete, onClose }: Props = $props();
	let envs = $state<{ key: string; value: string; editing: string }[]>([]);
	let params = $state<{ key: string; value: string }[]>([]);
	let input = $state<{ key: string; value: string }[]>([]);
	let testOutput = $state<Promise<{ output: string }>>();
	let advanced = $state(false);
	let dialog: HTMLDialogElement;
	let requestDelete = $state(false);
	let saver: SaveMonitor;
	const editorFile = $state<EditorItem>({
		id: '',
		name: '',
		file: {
			contents: '',
			buffer: ''
		}
	});

	onMount(async () => {
		if (!tool.instructions && tool.toolType) {
			tool.instructions = defaultInstructions[tool.toolType];
			if (Object.keys(tool.params ?? {}).length === 0) {
				tool.params = defaultParams;
			}
		}

		if (editorFile.file && tool.instructions) {
			editorFile.file.contents = tool.instructions;
		}

		if (tool.params) {
			params = fromMap(tool.params);
		}

		const envMap = await ChatService.getToolEnv(project.assistantID, project.id, tool.id);
		envs = fromMap(envMap).map((e) => ({ ...e, editing: e.value }));

		saver = newSaveMonitor(buildSaveState, saveState, commitState);
		saver.start();
	});

	onDestroy(() => {
		saver.stop();
	});

	$effect(() => {
		// Update input to match params keys with empty values
		input = params.map((p) => ({
			key: p.key,
			value: ''
		}));
	});

	function buildSaveState(): SaveState {
		tool.params = toMap(params);
		const env = toMap(envs);
		return {
			tool,
			env
		};
	}

	async function saveState(state: SaveState): Promise<SaveState> {
		await ChatService.updateTool(project.assistantID, project.id, state.tool, {
			env: state.env
		});
		return {
			tool: state.tool,
			env: state.env
		};
	}

	function commitState(state: SaveState) {
		if (state.tool.params) {
			params = fromMap(state.tool.params);
		}
		envs = fromMap(state.env).map((e) => ({ ...e, editing: e.value }));
		onSave(state.tool);
	}

	function toMap(values: { key: string; value: string }[]): Record<string, string> {
		return Object.fromEntries(values.map(({ key, value }) => [key, value]));
	}

	function fromMap(values: Record<string, string>): { key: string; value: string }[] {
		return Object.entries(values).map(([key, value]) => ({ key, value }));
	}

	function test(checkModal: boolean) {
		if (checkModal && !testOutput && params.length > 0) {
			dialog.showModal();
			return;
		}
		const testTool = { ...tool };
		testTool.params = toMap(params);

		testOutput = ChatService.testTool(
			project.assistantID,
			project.id,
			testTool,
			Object.fromEntries(input.map(({ key, value }) => [key, value])),
			{
				env: toMap(envs)
			}
		);
		if (!checkModal) {
			dialog.close();
		}
	}

	async function deleteTool() {
		await ChatService.deleteTool(project.assistantID, project.id, tool.id);
		await onDelete(tool);
		requestDelete = false;
	}
</script>

<div class="flex h-full flex-col">
	{#if responsive.isMobile}
		<h3 class="default-dialog-title" class:default-dialog-mobile-title={responsive.isMobile}>
			<input
				bind:value={tool.name}
				placeholder="Enter Name"
				class="w-full bg-inherit text-center text-xl font-semibold outline-none"
			/>
			<button class="icon-button mobile-header-button" onclick={() => onClose?.()}>
				<ChevronRight class="size-6" />
			</button>
			<div class="absolute top-1/2 left-5 -z-10 -translate-y-1/2">
				<SquarePen class="size-5 text-gray-500" />
			</div>
		</h3>
	{/if}

	{#if !responsive.isMobile}
		<div class="flex items-center justify-between gap-8 p-5 pr-3">
			<input
				bind:value={tool.name}
				placeholder="Enter Name"
				class="ghost-input dark:hover:border-surface3 w-full bg-inherit text-xl font-semibold outline-none"
			/>
			<div class="flex gap-2">
				{@render deleteButton()}
				<button class="icon-button h-fit" onclick={() => onClose?.()}>
					<X class="size-5" />
				</button>
			</div>
		</div>
	{/if}

	<div class="default-scrollbar-thin relative flex grow flex-col gap-5 overflow-y-auto p-5 pt-0">
		<div class="flex w-full justify-between gap-4">
			<textarea
				use:autoHeight
				rows="1"
				bind:value={tool.description}
				placeholder="Enter description (a good one is very helpful)"
				class="ghost-input dark:hover:border-surface3 w-full resize-none bg-inherit outline-none"
			></textarea>
			{#if responsive.isMobile}
				{@render deleteButton()}
			{/if}
		</div>

		<Params bind:params />

		<div
			class="bg-surface1 flex flex-col rounded-lg pb-1"
			class:pb-5={tool.toolType === 'container'}
		>
			<div class="flex items-center">
				<span class="flex-1 px-5 py-4 text-lg font-semibold">
					{#if tool.toolType === 'container'}
						Image
					{:else if tool.toolType === 'script'}
						Script
					{:else}
						Code
					{/if}
				</span>
			</div>
			<div class="flex w-full items-center gap-2" class:px-5={tool.toolType === 'container'}>
				{#if tool.toolType === 'container'}
					<Container class="size-5" />
					<input
						bind:value={tool.image}
						class="text-input-filled w-full"
						placeholder="Container image name"
					/>
				{:else}
					<Codemirror
						items={[]}
						class="default-scrollbar-thin m-0 max-h-[50vh] w-full overflow-y-auto border-r-2"
						file={editorFile}
						onFileChanged={(_, c) => {
							tool.instructions = c;
						}}
					/>
				{/if}
			</div>
		</div>

		{#if testOutput}
			<div class="bg-surface1 relative flex flex-col gap-4 rounded-lg p-5" transition:fade>
				<div class="absolute top-0 right-0 flex p-5">
					<button onclick={() => (testOutput = undefined)}>
						<X class="size-5" />
					</button>
				</div>
				<h4 class="mb-4 text-xl font-semibold">Output</h4>
				<Params bind:params bind:input classes={{ header: 'bg-surface1' }} />
				<div class="font-mono text-sm whitespace-pre-wrap">
					{#await testOutput}
						Running...
					{:then output}
						<div class="rounded-lg bg-white p-5 font-mono whitespace-pre-wrap dark:bg-black">
							{output.output}
						</div>
					{:catch error}
						{error}
					{/await}
				</div>
			</div>
		{/if}
		{#if advanced}
			<div transition:fade class="flex flex-col gap-5">
				<Env bind:envs />

				<div class="bg-surface1 flex flex-col gap-2 rounded-lg p-5">
					<h4 class="text-lg font-semibold">Calling Instructions</h4>
					<textarea
						bind:value={tool.context}
						use:autoHeight
						rows="1"
						class="text-input-filled resize-none"
						placeholder="(optional) More information on how or when AI should invoke this tool."
					></textarea>
				</div>

				{#if tool.toolType !== 'container'}
					<div class="bg-surface1 flex flex-col gap-4 rounded-lg p-5">
						<h4 class="text-lg font-semibold">Runtime Docker Image</h4>
						<div class="flex items-center gap-2">
							<Container class="size-5" />
							<input
								bind:value={tool.image}
								class="text-input-filled"
								placeholder="Container image name"
							/>
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</div>

	<div class="flex w-full flex-col items-center justify-between gap-4 p-5 md:flex-row">
		<button
			class="button-text flex items-center gap-2 p-0 text-xs md:text-sm"
			onclick={() => (advanced = !advanced)}
		>
			<span>{advanced ? 'Collapse' : 'Show'} Advanced Options...</span>
		</button>

		<button
			onclick={() => {
				test(true);
			}}
			class="button-primary w-full md:w-fit md:min-w-36"
		>
			Test
		</button>
	</div>
</div>

{#snippet deleteButton()}
	<button
		class="button-destructive h-fit p-2.5"
		onclick={() => (requestDelete = true)}
		use:tooltip={{ text: 'Delete Custom Tool', disablePortal: true }}
	>
		<Trash2 class="size-5" />
	</button>
{/snippet}

<dialog
	bind:this={dialog}
	use:clickOutside={() => dialog?.close()}
	class="max-w-full md:min-w-md"
	class:p-4={!responsive.isMobile}
	class:mobile-screen-dialog={responsive.isMobile}
>
	<h4 class="default-dialog-title" class:default-dialog-mobile-title={responsive.isMobile}>
		Input
		<button
			class="icon-button"
			class:mobile-header-button={responsive.isMobile}
			onclick={() => {
				dialog.close();
			}}
		>
			{#if responsive.isMobile}
				<ChevronRight class="size-6" />
			{:else}
				<X class="size-5" />
			{/if}
		</button>
	</h4>
	<div class="relative mt-5 flex flex-col">
		<Params bind:params autofocus bind:input />
		<button onclick={() => test(false)} class="button-primary mt-3 self-end"> Run </button>
	</div>
</dialog>

<Confirm
	msg="Are you sure you want to delete this tool?"
	show={requestDelete}
	oncancel={() => (requestDelete = false)}
	onsuccess={() => deleteTool()}
/>
