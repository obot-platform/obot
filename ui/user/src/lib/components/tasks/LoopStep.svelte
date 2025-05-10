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

	type Props = {
		value: string;
		messages: Messages;
		project: Project;
		isLoopStepRunning?: boolean;
		isStepRunning?: boolean;
		isStepRunned?: boolean;
		isReadOnly?: boolean;
		shouldShowOutput?: boolean;
		stale?: boolean;
		index?: number;
		onKeydown?: KeyboardEventHandler<HTMLTextAreaElement>;
		onDelete?: () => void;
		onAdd?: () => void;
	};

	let {
		value = $bindable(),
		messages,
		project,
		isLoopStepRunning = false,
		isStepRunning = false,
		isStepRunned = false,
		isReadOnly = false,
		shouldShowOutput = false,
		stale = false,
		index = 0,
		onKeydown = undefined,
		onDelete = undefined,
		onAdd = undefined
	}: Props = $props();
</script>

<div
	class="iteration-step flex w-full flex-col gap-2 transition-opacity duration-100"
	class:opacity-50={isStepRunning && !isLoopStepRunning}
>
	<div class="flex items-center gap-2 overflow-hidden">
		<textarea
			use:autoHeight
			bind:value
			rows="1"
			placeholder="Instructions..."
			class="ghost-input border-surface2 h-auto grow resize-none"
			disabled={isReadOnly}
			onkeydown={onKeydown}
		></textarea>

		{#if !isReadOnly}
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
			class="transition-height relative -ml-4 box-content flex min-h-6 flex-col gap-4 overflow-hidden rounded-lg bg-white p-5 duration-200 dark:bg-black"
			class:outline-2={isStepRunning && isLoopStepRunning}
			class:outline-blue={isStepRunning && isLoopStepRunning}
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
					{#each messages.messages as msg}
						{#if !msg.sent}
							<!-- automatically exapnd the message content when loop step is running -->
							<Message {msg} {project} disableMessageToEditor />
						{/if}
					{/each}
				{/if}
			</div>

			{#if stale}
				<div
					class="absolute inset-0 h-full w-full rounded-3xl bg-white opacity-80 dark:bg-black"
				></div>
			{/if}
		</div>
	{/if}
</div>
