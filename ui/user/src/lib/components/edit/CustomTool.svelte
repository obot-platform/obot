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
	import { Container, X, ChevronDown, ChevronUp } from 'lucide-svelte';
	import { type AssistantTool, ChatService, type Project } from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import Env from '$lib/components/edit/customtool/Env.svelte';
	import Params from '$lib/components/edit/customtool/Params.svelte';
	import Codemirror from '$lib/components/editor/Codemirror.svelte';
	import { Trash } from 'lucide-svelte/icons';
	import type { EditorItem } from '$lib/services/editor/index.svelte.js';
	import { onDestroy, onMount } from 'svelte';
	import { newSaveMonitor, type SaveMonitor } from '$lib/save';

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
		// compare the keys of params and input
		const inputKeys = new Set(input.map((i) => i.key));
		const paramKeys = new Set(params.map((p) => p.key));
		if (inputKeys.size !== paramKeys.size || [...inputKeys].some((key) => !paramKeys.has(key))) {
			// if they are different, set input to params
			input = params.map((p) => ({
				key: p.key,
				value: input.find((i) => i.key === p.key)?.value ?? ''
			}));
		}
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

<div class="relative flex flex-col gap-5 rounded-s-lg p-5">
	<div class="absolute top-0 right-0 m-2 flex">
		<button class="icon-button" onclick={() => (requestDelete = true)}>
			<Trash class="size-5" />
		</button>
		<button class="icon-button" onclick={() => onClose?.()}>
			<X class="size-5" />
		</button>
	</div>

	<div class="flex flex-col">
		<input
			bind:value={tool.name}
			placeholder="Enter Name"
			class="bg-inherit text-xl font-semibold outline-none"
		/>
		<textarea
			use:autoHeight
			bind:value={tool.description}
			placeholder="Enter description (a good one is very helpful)"
			class="resize-none bg-inherit outline-none"
		></textarea>
	</div>

	<Params bind:params />

	<div class="bg-surface1 flex flex-col gap-4 rounded-lg p-5">
		<div class="flex items-center">
			<span class="flex-1 text-lg font-semibold">
				{#if tool.toolType === 'container'}
					Image
				{:else if tool.toolType === 'script'}
					Script
				{:else}
					Code
				{/if}
			</span>
		</div>
		<div class="flex w-full items-center gap-2">
			{#if tool.toolType === 'container'}
				<Container class="size-5" />
				<input bind:value={tool.image} class="text-input" placeholder="Container image name" />
			{:else}
				<Codemirror
					items={[]}
					class="w-full"
					file={editorFile}
					onFileChanged={(_, c) => {
						tool.instructions = c;
					}}
				/>
			{/if}
		</div>
		<button
			onclick={() => {
				test(true);
			}}
			class="button-primary mt-3 self-end"
		>
			Test
		</button>
	</div>

	{#if testOutput}
		<div class="bg-surface1 relative flex flex-col gap-4 rounded-lg p-5">
			<div class="absolute top-0 right-0 flex p-5">
				<button onclick={() => (testOutput = undefined)}>
					<X class="size-5" />
				</button>
			</div>
			<h4 class="text-xl font-semibold">Output</h4>
			<Params bind:params={input} input />
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

	<button
		class="dark:text-gray flex items-center gap-2 self-end"
		onclick={() => (advanced = !advanced)}
	>
		<span>Advanced Options</span>
		{#if advanced}
			<ChevronUp class="size-5" />
		{:else}
			<ChevronDown class="size-5" />
		{/if}
	</button>

	<div class:contents={advanced} class:hidden={!advanced}>
		<Env bind:envs />

		<div class="bg-surface1 flex flex-col gap-2 rounded-lg p-5">
			<h4 class="text-xl font-semibold">Calling Instructions</h4>
			<textarea
				bind:value={tool.context}
				use:autoHeight
				rows="1"
				class="resize-none bg-gray-50 outline-none dark:bg-gray-950"
				placeholder="(optional) More information on how or when AI should invoke this tool."
			></textarea>
		</div>

		{#if tool.toolType !== 'container'}
			<div class="bg-surface1 flex flex-col gap-4 rounded-lg p-5">
				<h4 class="text-xl font-semibold">Runtime Docker Image</h4>
				<div class="flex items-center gap-2">
					<Container class="size-5" />
					<input bind:value={tool.image} class="text-input" placeholder="Container image name" />
				</div>
			</div>
		{/if}
	</div>
</div>

<dialog bind:this={dialog} class="w-11/12 max-w-[1000px]">
	<div class="relative flex flex-col p-5">
		<div class="absolute top-0 right-0 flex p-5">
			<button
				onclick={() => {
					dialog.close();
				}}
			>
				<X class="size-5" />
			</button>
		</div>
		<h4 class="mb-2 text-xl font-semibold">Input</h4>
		<Params bind:params={input} autofocus input />
		<button onclick={() => test(false)} class="button-primary mt-3 self-end"> Run </button>
	</div>
</dialog>

<Confirm
	msg="Are you sure you want to delete this tool?"
	show={requestDelete}
	oncancel={() => (requestDelete = false)}
	onsuccess={() => deleteTool()}
/>
