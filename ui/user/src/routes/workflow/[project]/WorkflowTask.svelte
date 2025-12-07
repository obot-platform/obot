<script lang="ts">
	import MarkdownTextEditor from '$lib/components/admin/MarkdownTextEditor.svelte';
	import { createVariablePillPlugin } from './variablePillPlugin';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { ReceiptText, Trash2 } from 'lucide-svelte';

	type WorkflowTask = {
		id: string;
		name: string;
		description: string;
		content: string;
	};

	interface Props {
		task: {
			id: string;
			name: string;
			description: string;
			content: string;
		};
		onVariableAddition?: (variable: string) => void;
		onVariableDeletion?: (variable: string) => void;
		onDelete?: (task: WorkflowTask) => void;
	}

	let { task = $bindable(), onVariableAddition, onVariableDeletion, onDelete }: Props = $props();

	let showDescription = $state(task.description.trim().length > 0);

	// Create plugin once - use a wrapper function to always call the current callback
	const variablePillPlugin = createVariablePillPlugin({
		onVariableAddition: (variable: string) => onVariableAddition?.(variable),
		onVariableDeletion: (variable: string) => onVariableDeletion?.(variable)
	});
</script>

<div class="flex flex-col gap-1 pr-12">
	<input
		class="ghost-input relative z-20 text-lg font-semibold"
		bind:value={task.name}
		placeholder="Task title"
	/>
	{#if showDescription}
		<div class="my-2">
			<MarkdownTextEditor placeholder="Add description..." bind:value={task.description} />
		</div>
	{/if}
	<div class="my-2">
		<MarkdownTextEditor
			placeholder="Add content..."
			bind:value={task.content}
			plugins={[variablePillPlugin]}
		/>
	</div>
</div>

<DotDotDot
	disablePortal
	class="hover:text-primary hover:bg-primary/10 absolute top-4 right-4 z-100 rounded-full p-2 opacity-0 transition-colors group-hover:opacity-100"
>
	<div class="default-dialog flex min-w-48 flex-col p-2">
		<div
			class="flex items-center justify-between gap-2 p-2"
			role="none"
			onclick={(e) => e.stopPropagation()}
		>
			<span class="flex items-center gap-2"><ReceiptText class="size-4" /> Description</span>
			<Toggle
				label=""
				labelInline
				checked={showDescription}
				onChange={(checked) => {
					showDescription = checked;
				}}
			/>
		</div>
		<button
			class="menu-button"
			onclick={() => {
				onDelete?.(task);
			}}
		>
			<Trash2 class="size-4" /> Delete task
		</button>
	</div>
</DotDotDot>
