<script lang="ts">
	import type { Messages, Project } from '$lib/services';
	import Message from '$lib/components/messages/Message.svelte';
	import { Trash2, Plus } from 'lucide-svelte/icons';
	import { autoHeight } from '$lib/actions/textarea.js';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { KeyboardEventHandler } from 'svelte/elements';
	import { transitionParentHeight } from '$lib/actions/size.svelte';
	import { slide } from 'svelte/transition';
	import { linear } from 'svelte/easing';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		class?: string;
		value: string;
		messages: Messages;
		project: Project;
		isLoopStepRunning?: boolean;
		isStepRunning?: boolean;
		isStepRunned?: boolean;
		isTaskRunning?: boolean;
		isReadOnly?: boolean;
		shouldShowOutput?: boolean;
		index?: number;
		onKeydown?: KeyboardEventHandler<HTMLTextAreaElement>;
		onDelete?: () => void;
		onAdd?: () => void;
	};

	let {
		value = $bindable(),
		class: klass = '',
		messages,
		project,
		isLoopStepRunning = false,
		isStepRunning = false,
		isStepRunned = false,
		isTaskRunning = false,
		isReadOnly = false,
		shouldShowOutput = false,
		index = 0,
		onKeydown = undefined,
		onDelete = undefined,
		onAdd = undefined
	}: Props = $props();
</script>

<div
	class={twMerge(
		'iteration-step flex w-full flex-col gap-0 transition-opacity duration-100',
		klass
	)}
	class:opacity-50={isStepRunning && !isLoopStepRunning}
	class:outline-2={isStepRunning && isLoopStepRunning}
	class:outline-blue={isStepRunning && isLoopStepRunning}
>
	<div class="flex items-center gap-2 overflow-hidden pl-4">
		<textarea
			use:autoHeight
			{value}
			rows="1"
			placeholder="Instructions..."
			class="ghost-input border-surface2 h-auto grow resize-none"
			disabled={isReadOnly}
			readonly={isReadOnly || isTaskRunning}
			onkeydown={onKeydown}
			oninput={(ev) => {
				value = (ev.target as HTMLInputElement).value;
			}}
		></textarea>

		{#if !isReadOnly && !isStepRunned && !isStepRunning}
			<div class="flex items-center">
				<button class="icon-button" onclick={onDelete} use:tooltip={'Remove step from loop'}>
					<Trash2 class="size-4" />
				</button>

				<button class="icon-button self-start" onclick={onAdd} use:tooltip={'Add step to loop'}>
					<Plus class="size-4" />
				</button>
			</div>
		{/if}
	</div>

	{#if (isStepRunning || isStepRunned) && shouldShowOutput}
		<div
			class="loop-step-messages transition-loop-step-message relative box-content flex min-h-11 flex-col gap-4 overflow-hidden rounded-none px-4 py-4 duration-200"
			in:slide|global={{
				duration: !isReadOnly ? 200 : 0,
				delay: isStepRunning && !isReadOnly ? index * 190 : 0,
				easing: linear
			}}
			out:slide={{
				duration: 200,
				delay: 0,
				easing: linear
			}}
		>
			<div
				class="messages-list flex h-fit w-full flex-col gap-4"
				use:transitionParentHeight={() => (isStepRunning && shouldShowOutput) || messages.messages}
			>
				{#if messages.messages?.length > 0}
					{#each messages.messages as msg, i (i)}
						{#if !msg.sent}
							<!-- automatically exapnd the message content when loop step is running -->
							<Message {msg} {project} disableMessageToEditor />
						{/if}
					{/each}
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	:global(.transition-loop-step-message) {
		transition-property: height, transform, translate, margin;
		transition-duration: var(--tw-duration, 100ms);
		transition-timing-function: var(--tw-timing-function, linear);

		will-change: height, transform, translate, margin;
		transform: translateZ(1);
	}
</style>
