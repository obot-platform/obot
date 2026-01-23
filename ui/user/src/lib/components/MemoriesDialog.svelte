<script lang="ts">
	import {
		type Project,
		type Memory,
		getMemories,
		deleteAllMemories,
		deleteMemory,
		updateMemory
	} from '$lib/services';
	import { Trash2, RefreshCcw, Edit, Check, X as XIcon, Pencil } from 'lucide-svelte/icons';
	import { fade } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import errors from '$lib/stores/errors.svelte';
	import Confirm from './Confirm.svelte';
	import { onMount, tick } from 'svelte';
	import { twMerge } from 'tailwind-merge';
	import DotDotDot from './DotDotDot.svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import ResponsiveDialog from './ResponsiveDialog.svelte';
	import Table from './table/Table.svelte';

	interface Props {
		project?: Project;
		showPreview?: boolean;
	}

	let { project = $bindable(), showPreview }: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let memories = $state<Memory[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let toDeleteAll = $state(false);
	let editingMemoryId = $state<string | null>(null);
	let editContent = $state('');
	let editingPreview = $state(false);
	let input = $state<HTMLTextAreaElement>();
	let deleteMemoryId = $state<string>();

	export function show(projectToUse?: Project) {
		if (projectToUse) {
			project = projectToUse;
		}

		dialog?.open();
		loadMemories();
	}

	async function loadMemories() {
		if (!project) return;

		loading = true;
		error = null;
		try {
			const result = await getMemories(project.assistantID, project.id);
			memories = result.items || [];
		} catch (err) {
			// Ignore 404 errors (memory tool not configured or no memories)
			if (err instanceof Error && err.message.includes('404')) {
				memories = [];
			} else {
				// For all other errors, append to errors store
				errors.append(err);
				error = 'Failed to load memories';
			}
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		if (showPreview && project) {
			loadMemories();
		}
	});

	async function deleteAll() {
		if (!project) return;

		loading = true;
		error = null;
		try {
			await deleteAllMemories(project.assistantID, project.id);
			memories = [];
		} catch (err) {
			errors.append(err);
			error = 'Failed to delete all memories';
		} finally {
			loading = false;
			toDeleteAll = false;
		}
	}

	async function deleteOne(memoryId: string) {
		if (!project) return;

		loading = true;
		error = null;
		try {
			await deleteMemory(project.assistantID, project.id, memoryId);
			memories = memories.filter((memory) => memory.id !== memoryId);
		} catch (err) {
			errors.append(err);
			error = 'Failed to delete memory';
		} finally {
			loading = false;
		}
	}

	function startEdit(memory: Memory, inPreview?: boolean) {
		editingMemoryId = memory.id;
		editContent = memory.content;
		editingPreview = inPreview ?? false;

		tick().then(() => {
			input?.focus();
			input?.setSelectionRange(0, 0);
		});
	}

	function cancelEdit() {
		editingMemoryId = null;
		editContent = '';
	}

	async function saveEdit() {
		if (!project || !editingMemoryId) return;

		loading = true;
		error = null;
		try {
			const updatedMemory = await updateMemory(
				project.assistantID,
				project.id,
				editingMemoryId,
				editContent
			);
			// Update the memory in the list
			memories = memories.map((memory) => (memory.id === editingMemoryId ? updatedMemory : memory));
			editingMemoryId = null;
			editContent = '';
		} catch (err) {
			errors.append(err);
			error = 'Failed to update memory';
		} finally {
			loading = false;
		}
	}

	function formatDate(dateString: string): string {
		if (!dateString) return '';

		try {
			const date = new Date(dateString);
			return date.toLocaleString();
		} catch (_e) {
			return '';
		}
	}

	export async function viewAllMemories() {
		dialog?.open();
	}

	export function refresh() {
		loadMemories();
	}
</script>

{#if showPreview}
	<div class="flex h-full grow flex-col gap-2">
		{@render content(true)}
	</div>
{/if}

<ResponsiveDialog title="Memories" bind:this={dialog}>
	<div class="flex w-full flex-col gap-4 p-4 md:p-0">
		{@render content()}
	</div>
</ResponsiveDialog>

{#snippet content(preview = false)}
	{#if error}
		<div class="rounded bg-red-100 p-3 text-red-800">{error}</div>
	{/if}
	{#if !preview}
		<div class="flex items-center justify-between">
			<span class="text-text2 text-sm">{memories.length} memories</span>
			<div class="flex gap-2">
				<button class="icon-button" onclick={() => loadMemories()} use:tooltip={'Refresh Memories'}>
					<RefreshCcw class="size-4" />
				</button>

				{@render deleteAllButton(preview)}
			</div>
		</div>
	{/if}

	<div class="min-h-0 flex-1 overflow-auto">
		{#if loading}
			<div in:fade class="flex justify-center py-10">
				<div
					class="border-primary h-8 w-8 animate-spin rounded-full border-4 border-t-transparent"
				></div>
			</div>
		{:else if memories.length === 0 && !preview}
			<p in:fade class="text-on-surface1 pt-6 pb-3 text-center text-sm" class:text-xs={preview}>
				No memories stored
			</p>
		{:else if !preview}
			<div class="overflow-auto">
				<Table
					fields={['createdAt', 'content']}
					headers={[
						{
							title: 'Created',
							property: 'createdAt'
						}
					]}
					data={memories}
					classes={{
						root: 'bg-surface1 dark:bg-background'
					}}
				>
					{#snippet onRenderColumn(field, memory)}
						{#if field === 'createdAt'}
							{formatDate(memory.createdAt)}
						{:else}
							{@render memoryContent(memory, true)}
						{/if}
					{/snippet}
					{#snippet actions(memory)}
						{@render options(memory, preview)}
					{/snippet}
				</Table>
			</div>
		{:else}
			<div class="flex w-full flex-col gap-4">
				{#each memories as memory (memory.id)}
					<div
						class="text-md dark:bg-surface1 dark:border-surface3 bg-background flex items-center justify-between gap-4 rounded-md border border-transparent p-4 shadow-sm"
					>
						{#if editingMemoryId === memory.id}
							<div class="flex w-full flex-col gap-4">
								<textarea
									use:autoHeight
									bind:value={editContent}
									onkeyup={(e) => {
										switch (e.key) {
											case 'Escape':
												cancelEdit();
												break;
										}
									}}
									bind:this={input}
									class="default-scrollbar-thin text-on-background min-h-10 w-full border-none bg-transparent pr-0 ring-0 outline-hidden"
								></textarea>
								<div class="flex justify-end gap-4">
									<button class="button text-xs" onclick={cancelEdit}> Cancel </button>
									<button class="button-primary text-xs" onclick={saveEdit}> Save </button>
								</div>
							</div>
						{:else}
							<p>
								{memory.content}
							</p>
						{/if}
						{#if editingMemoryId !== memory.id}
							<DotDotDot class="hover:text-on-background text-on-surface1  p-0">
								<button class="menu-button" onclick={() => startEdit(memory, true)}>
									<Pencil class="size-4" /> Edit
								</button>
								<button
									class="menu-button text-red-500"
									onclick={() => (deleteMemoryId = memory.id)}
								>
									<Trash2 class="size-4" /> Delete
								</button>
							</DotDotDot>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>
{/snippet}

{#snippet memoryContent(memory: Memory, preview: boolean)}
	{#if editingMemoryId === memory.id && preview === editingPreview}
		<textarea
			bind:value={editContent}
			class="text-input-filled border-surface1 bg-background min-h-[80px] w-full resize-none border"
			rows="3"
		></textarea>
	{:else}
		<p class="flex grow">
			{memory.content}
		</p>
	{/if}
{/snippet}

{#snippet options(memory: Memory, inline: boolean)}
	{#if editingMemoryId === memory.id && inline === editingPreview}
		<button
			class={twMerge('icon-button text-green-500', inline && 'min-h-auto min-w-auto p-1.5')}
			onclick={saveEdit}
			use:tooltip={'Save changes'}
		>
			<Check class="size-4" />
		</button>
		<button
			class={twMerge('icon-button text-red-500', inline && 'min-h-auto min-w-auto p-1.5')}
			onclick={cancelEdit}
			use:tooltip={'Cancel'}
		>
			<XIcon class="size-4" />
		</button>
	{:else}
		<button
			class={twMerge('icon-button', inline && 'min-h-auto min-w-auto p-1.5')}
			onclick={() => startEdit(memory, inline)}
			disabled={loading}
			use:tooltip={'Edit memory'}
		>
			<Edit class="size-4" />
		</button>
		<button
			class={twMerge('icon-button', inline && 'min-h-auto min-w-auto p-1.5')}
			onclick={() => (deleteMemoryId = memory.id)}
			disabled={loading}
			use:tooltip={'Delete memory'}
		>
			<Trash2 class="size-4" />
		</button>
	{/if}
{/snippet}

{#snippet deleteAllButton(inline?: boolean)}
	<button
		class={twMerge('button-destructive', inline && 'py-2 text-xs')}
		onclick={() => (toDeleteAll = true)}
		disabled={loading || memories.length === 0}
	>
		<Trash2 class="size-4" />
		Delete All
	</button>
{/snippet}

<Confirm
	msg="Delete all memories?"
	show={toDeleteAll}
	onsuccess={deleteAll}
	oncancel={() => (toDeleteAll = false)}
/>

<Confirm
	msg={`Delete ${deleteMemoryId}?`}
	show={!!deleteMemoryId}
	onsuccess={() => {
		if (deleteMemoryId) {
			deleteOne(deleteMemoryId);
			deleteMemoryId = undefined;
		}
	}}
	oncancel={() => (deleteMemoryId = undefined)}
/>

<style lang="postcss">
	.memory {
		border-left: 5px solid var(--color-primary);
		background-color: var(--color-white);
		color: black;
		font-size: 0.8em;
		padding: 0.5rem;
		cursor: default;
		position: relative;
		max-width: calc(100% - 30px);
	}

	:global(.dark) .memory {
		color: white;
		background-color: var(--color-surface2);
	}

	:global(.dark) .memory::before,
	:global(.dark) .memory::after {
		background-color: var(--color-surface2);
	}

	.memory::before {
		content: '';
		position: absolute;
		top: calc(50% - 15px);
		transform: translateY(-50%);
		right: -20px;
		width: 10px;
		height: 10px;
		background-color: var(--color-white);
		border-radius: 50%;
	}

	.memory::after {
		content: '';
		position: absolute;
		top: 50%;
		transform: translateY(-50%);
		right: -10px;
		width: 20px;
		height: 20px;
		background-color: var(--color-white);
		border-radius: 50%;
	}
</style>
