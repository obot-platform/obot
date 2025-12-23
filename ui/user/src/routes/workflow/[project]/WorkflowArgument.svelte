<script lang="ts">
	import MarkdownTextEditor from '$lib/components/admin/MarkdownTextEditor.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { ReceiptText, Trash2 } from 'lucide-svelte';

	interface Argument {
		name: string;
		displayLabel: string;
		description: string;
		id: string;
		visible: boolean;
	}

	interface Props {
		arg: Argument;
		onDelete?: (arg: Argument) => void;
	}

	let { arg = $bindable(), onDelete }: Props = $props();
	let showDescription = $state(arg.description.trim().length > 0);
</script>

<div class="flex min-w-0 flex-1 flex-col gap-1 p-4">
	<div class="flex items-center gap-2">
		<div
			class="bg-primary/10 text-primary relative z-10 flex w-[calc(100%-4rem)] items-center rounded-lg"
		>
			<label for={`argument-name-${arg.id}`} class="flex px-2 font-medium">$</label>
			<input
				id={`argument-name-${arg.id}`}
				placeholder="Argument"
				bind:value={arg.name}
				class="ghost-input text-primary placeholder:text-primary/50 flex grow font-medium"
			/>
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
						onDelete?.(arg);
					}}
				>
					<Trash2 class="size-4" /> Delete details
				</button>
			</div>
		</DotDotDot>
	</div>
	<div class="flex flex-col gap-0.5 px-2">
		<input
			class="ghost-input text-md relative z-10 font-semibold"
			bind:value={arg.displayLabel}
			placeholder="Display Label"
		/>
		{#if showDescription}
			<div class="relative z-10 my-2">
				<MarkdownTextEditor placeholder="Add description..." bind:value={arg.description} />
			</div>
		{/if}
	</div>
</div>
