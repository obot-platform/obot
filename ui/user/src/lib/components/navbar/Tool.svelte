<script lang="ts">
	import { autoHeight, resize } from '$lib/actions/textarea.js';
	import { Container, Minus, X, Plus, Trash2 } from 'lucide-svelte';
	import { type AssistantTool, ChatService } from '$lib/services';
	import { currentAssistant } from '$lib/stores';
	import Confirm from '$lib/components/Confirm.svelte';
	import { tick } from 'svelte';

	interface Props {
		id?: string;
		onCancel?: () => void;
	}

	let { id, onCancel }: Props = $props();

	const masked = '•••••••';
	let name = $state<string>();
	let image = $state<string>();
	let context = $state<string>();
	let description = $state<string>();
	let params: { key: string; value: string }[] = $state([]);
	let envs: { key: string; value: string; editing: string }[] = $state([]);
	let requestDelete = $state(false);
	let loaded = load();

	export async function deleteTool() {
		requestDelete = false;
		if (!id) {
			return;
		}
		await ChatService.deleteTool($currentAssistant.id, id);
		onCancel?.();
	}

	export async function save() {
		if (!name || !image) {
			return;
		}
		const tool: AssistantTool = {
			id: id ?? '',
			name,
			description,
			instructions: image,
			context,
			params: params.reduce(
				(acc, { key, value }) => {
					acc[key] = value;
					return acc;
				},
				{} as Record<string, string>
			)
		};
		const newEnv: Record<string, string> = envs.reduce(
			(acc, { key, value }) => {
				acc[key] = value;
				return acc;
			},
			{} as Record<string, string>
		);
		if (id) {
			await ChatService.updateTool($currentAssistant.id, tool, { env: newEnv });
		} else {
			await ChatService.createTool($currentAssistant.id, tool, { env: newEnv });
		}
		onCancel?.();
	}

	export async function load() {
		if (!id || typeof window === 'undefined') {
			return;
		}
		const tool = await ChatService.getTool($currentAssistant.id, id);
		const env = await ChatService.getToolEnv($currentAssistant.id, id);
		name = tool.name;
		description = tool.description;
		image = tool.instructions;
		context = tool.context;
		params = Object.entries(tool.params ?? {}).map(([key, value]) => ({ key, value }));
		envs = Object.entries(env).map(([key, value]) => ({ key, value, editing: masked }));
	}
</script>

<div class="relative flex flex-col gap-5">
	<h4 class="text-xl font-semibold">Docker Tool</h4>
	<div class="absolute right-0 top-0 flex gap-2">
		{#if id}
			<button onclick={() => (requestDelete = true)}>
				<Trash2 class="h-5 w-5" />
			</button>
		{/if}
		<button onclick={() => onCancel?.()}>
			<X class="h-5 w-5" />
		</button>
	</div>

	{#await loaded then}
		<div class="flex flex-col rounded-3xl bg-gray-50 p-5 dark:bg-gray-950">
			<input
				bind:value={name}
				placeholder="Enter Name"
				class="bg-gray-50 text-xl font-semibold outline-none dark:bg-gray-950"
			/>
			<input
				bind:value={description}
				placeholder="Enter description (a good one is very helpful)"
				class="bg-gray-50 outline-none dark:bg-gray-950"
			/>
		</div>

		<div class="flex gap-2 rounded-3xl bg-gray-50 p-5 dark:bg-gray-950">
			<Container class="h-5 w-5" />
			<input
				bind:value={image}
				class="bg-gray-50 outline-none dark:bg-gray-950"
				placeholder="Container image name"
			/>
		</div>

		<div class="flex flex-col gap-4 rounded-3xl bg-gray-50 p-5 dark:bg-gray-950">
			<div class="flex">
				<h4 class="flex-1 text-xl font-semibold">Parameters</h4>
				<button onclick={() => params.push({ key: '', value: '' })}>
					<Plus class="h-5 w-5" />
				</button>
			</div>
			{#if params.length !== 0}
				<table class="w-full table-auto text-left">
					<thead>
						<tr>
							<th>Name</th>
							<th>Description</th>
						</tr>
					</thead>
					<tbody>
						{#each params as param, i}
							<tr>
								<td
									><input
										bind:value={param.key}
										placeholder="Enter name"
										class="ast bg-gray-50 outline-none dark:bg-gray-950"
									/></td
								>
								<td
									><textarea
										use:autoHeight
										class="resize-none bg-gray-50 outline-none dark:bg-gray-950"
										rows="1"
										bind:value={param.value}
									></textarea></td
								>
								<td>
									<button onclick={() => params.splice(i, 1)}>
										<Minus class="h-5 w-5" />
									</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</div>

		<div class="flex flex-col gap-4 rounded-3xl bg-gray-50 p-5 dark:bg-gray-950">
			<div class="flex">
				<h4 class="flex-1 text-xl font-semibold">Environment Variables</h4>
				<button onclick={() => envs.push({ key: '', value: '', editing: '' })}>
					<Plus class="h-5 w-5" />
				</button>
			</div>
			{#if envs.length !== 0}
				<table class="w-full text-left">
					<thead>
						<tr>
							<th>Name</th>
							<th>Value</th>
						</tr>
					</thead>
					<tbody>
						{#each envs as env, i}
							<tr>
								<td
									><input
										bind:value={env.key}
										placeholder="Enter name"
										class="ast bg-gray-50 outline-none dark:bg-gray-950"
									/></td
								>
								<td
									><textarea
										use:autoHeight
										placeholder="Enter value"
										class="resize-none bg-gray-50 outline-none dark:bg-gray-950"
										rows="1"
										bind:value={env.editing}
										onfocusin={(e) => {
											if (env.editing === masked) {
												env.editing = env.value;
												const t = e.target;
												if (t instanceof HTMLTextAreaElement) {
													tick().then(() => resize(t));
												}
											}
										}}
										onfocusout={(e) => {
											if (env.editing !== masked) {
												env.value = env.editing;
												env.editing = masked;
												const t = e.target;
												if (t instanceof HTMLTextAreaElement) {
													tick().then(() => resize(t));
												}
											}
										}}
									></textarea></td
								>
								<td>
									<button onclick={() => envs.splice(i, 1)}>
										<Minus class="h-5 w-5" />
									</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</div>

		<div class="flex flex-col gap-2 rounded-3xl bg-gray-50 p-5 dark:bg-gray-950">
			<h4 class="text-xl font-semibold">Calling Instructions</h4>
			<textarea
				onchange={() => console.log('changed')}
				onfocus={() => console.log('changed')}
				bind:value={context}
				use:autoHeight
				rows="2"
				class="resize-none bg-gray-50 outline-none dark:bg-gray-950"
				placeholder="(optional) More information on how or when AI should invoke this tool."
			></textarea>
		</div>

		<div class="flex justify-end gap-2">
			<button
				onclick={() => onCancel?.()}
				class="mt-3 rounded-3xl bg-gray-50 p-3 px-6 dark:bg-gray-950"
			>
				Cancel
			</button>
			<button
				onclick={() => save()}
				class="mt-3 gap-2 rounded-3xl bg-blue p-3 px-6 text-white hover:bg-blue-400 hover:text-white"
			>
				{#if id}
					Save
				{:else}
					Create
				{/if}
			</button>
		</div>
	{/await}
</div>

<Confirm
	msg="Are you sure you want to delete this tool?"
	show={requestDelete}
	oncancel={() => (requestDelete = false)}
	onsuccess={() => deleteTool()}
/>
