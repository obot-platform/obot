<script lang="ts">
	import { autoHeight } from '$lib/actions/textarea';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { Message as MessageType, Messages, Project, TaskStep } from '$lib/services';
	import { ArrowLeft, ArrowRight, GripVertical, Plus, Trash2 } from 'lucide-svelte';
	import Message from '$lib/components/messages/Message.svelte';
	import { slide } from 'svelte/transition';

	interface Props {
		pIndex: number;
		step: TaskStep;
		loop?: string[];
		onkeydown: (e: KeyboardEvent) => void;
		showOutput?: boolean;
		stepMessages?: Map<string, Messages>;
		readOnly?: boolean;
		running?: boolean;
		project: Project;
	}

	let {
		pIndex,
		step,
		loop,
		onkeydown,
		showOutput,
		stepMessages,
		readOnly,
		running,
		project
	}: Props = $props();

	console.log('--- SubSteps ---', loop?.length, stepMessages);

	let paginationArr = $state(loop?.map((_) => 0) ?? []);
	let substepMessages = $state(formatedArray(loop, stepMessages));

	$effect(() => {
		// Update the substepMessages whenever loop or stepMessages change
		substepMessages = formatedArray(loop, stepMessages);
	});

	function formatedArray(
		loop: string[] | undefined,
		stepMessages: Map<string, Messages> | undefined
	) {
		let arr =
			loop?.map((_, i) => {
				const messages = Array.from(stepMessages ?? new Map())
					.filter(
						([key, value]) =>
							key.includes(step.id) &&
							key.includes(`step=`) &&
							value.messages[0]?.message.toString().includes(loop[i]) &&
							loop[i] !== ''
					)
					.map(([_, msg]) => msg.messages.filter((msg: MessageType) => !msg?.sent)[0]);
				return messages;
			}) ?? [];
		return arr;
	}
	// console.log('--- substepMessages3 ---', substepMessages);

	function removeSubStep(i: number) {
		loop!.splice(i, 1);
		paginationArr.splice(i, 1);
		substepMessages!.splice(i, 1);
	}
	function addSubStep(i: number) {
		loop!.splice(i + 1, 0, '');
		paginationArr.splice(i + 1, 0, 0);
		substepMessages.splice(i + 1, 0, []);
	}

	// Drag, Drop

	let draggingIndex = $state<number | null>(null);
	let hoveredIndex = $state<number | null>(null);

	function handleDragStart(i: number) {
		draggingIndex = i;
	}

	function handleDragOver(e: DragEvent, i: number) {
		e.preventDefault();
		hoveredIndex = i;
		if (draggingIndex !== null && draggingIndex !== i) {
			hoveredIndex = i;
		}
	}

	function handleDrop() {
		if (draggingIndex !== null && hoveredIndex !== null && draggingIndex !== hoveredIndex) {
			const movedLoop = loop?.[draggingIndex] ?? '';
			loop?.splice(draggingIndex, 1);
			loop?.splice(hoveredIndex, 0, movedLoop);
		}
		resetDragState();
	}

	function handleDragEnd() {
		resetDragState();
	}

	function resetDragState() {
		draggingIndex = null;
		hoveredIndex = null;
	}
</script>

<div>
	{#each loop! as _, i (i)}
		<div
			class="flex flex-col gap-2"
			class:drop-target={hoveredIndex === i && draggingIndex !== i}
			class:dragging={draggingIndex === i}
			class:dragging-over={hoveredIndex === i}
			class:dragging-into={draggingIndex !== null && draggingIndex !== i}
			draggable={!readOnly}
			role="listitem"
			ondragstart={() => handleDragStart(i)}
			ondragover={(e) => handleDragOver(e, i)}
			ondrop={() => handleDrop()}
			ondragend={handleDragEnd}
		>
			<div class="flex items-center gap-2">
				{pIndex + 1}.{i + 1}
				<textarea
					{onkeydown}
					rows="1"
					placeholder="Instructions..."
					use:autoHeight
					bind:value={loop![i]}
					class="ghost-input border-surface2 grow resize-none"
					disabled={readOnly}
				></textarea>
				{#if !readOnly}
					<div class="flex items-center">
						<button
							class="icon-button"
							onclick={() => removeSubStep(i)}
							use:tooltip={'Remove step from loop'}
						>
							<Trash2 class="size-4" />
						</button>
						<button
							class="icon-button self-start"
							onclick={() => addSubStep(i)}
							use:tooltip={'Add step to loop'}
						>
							<Plus class="size-4" />
						</button>
						<div
							class="flex items-center justify-center rounded-full p-2 text-gray-500 hover:cursor-grab hover:bg-gray-200 active:cursor-grabbing"
						>
							<GripVertical class="size-6" />
						</div>
					</div>
				{/if}
			</div>
			{#if (substepMessages[i].length > 0 || running) && showOutput}
				<div
					class="relative my-3 -ml-4 flex min-h-[150px] flex-col gap-4 rounded-lg bg-white p-5 transition-transform dark:bg-black"
					class:border-2={running}
					class:border-blue={running}
					transition:slide
				>
					{#each substepMessages[i] as msg, index}
						{#if paginationArr[i] === index}
							<Message
								msg={{ ...msg, sent: false }}
								{project}
								disableMessageToEditor
								maxHeight={'300px'}
							/>
						{/if}
					{/each}
					<div class="absolute right-2 bottom-2 mb-2 flex items-center justify-end gap-1 p-2">
						<button
							onclick={() =>
								paginationArr &&
								(paginationArr[i] =
									(paginationArr[i] - 1 + substepMessages[i].length) % substepMessages[i].length)}
							disabled={(paginationArr?.[i] ?? 0) === 0}
							class="rounded-md p-1 opacity-100 hover:bg-gray-300 disabled:opacity-50"
						>
							<ArrowLeft class="size-4" />
						</button>
						<select
							onchange={(e) => {
								if (e.target)
									(paginationArr ??= [])[i] = Number((e.target as HTMLSelectElement).value);
							}}
							class="flex appearance-none items-center justify-center border px-2 text-sm"
							value={paginationArr?.[i]}
						>
							{#each substepMessages[i] as _, index}
								<option value={index}>
									{index + 1}
								</option>
							{/each}
						</select>
						<button
							onclick={() =>
								((paginationArr ??= [])[i] =
									((paginationArr[i] ?? 0) + 1) % substepMessages[i].length)}
							disabled={(paginationArr?.[i] ?? 0) === substepMessages[i].length - 1}
							class="rounded-md p-1 opacity-100 hover:bg-gray-300 disabled:opacity-50"
						>
							<ArrowRight class="size-4" />
						</button>
					</div>
				</div>
			{/if}
		</div>
	{/each}
</div>

<style>
	.drop-target {
		background-color: var(--color-blue-100);
	}
	.dragging {
		background-color: var(--color-blue-200);
	}
	.dragging-over {
		background-color: var(--color-blue-300);
	}
</style>
