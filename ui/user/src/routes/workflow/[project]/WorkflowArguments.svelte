<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import {
		DraggableList,
		DraggableItem,
		DraggableHandle
	} from '$lib/components/primitives/draggable';
	import WorkflowArgument from './WorkflowArgument.svelte';
	import { Plus } from 'lucide-svelte';

	interface Argument {
		name: string;
		displayLabel: string;
		description: string;
		id: string;
		visible: boolean;
	}

	interface Props {
		args: Argument[];
		onDelete?: (arg: Argument) => void;
	}

	let { args = $bindable(), onDelete }: Props = $props();
</script>

<DraggableList
	as="ol"
	class="gap-6 pl-20"
	gap={12}
	order={args.map((item) => item.id)}
	onChange={(items) => {
		args = items as Argument[];
	}}
>
	{#each args as arg, index (arg.id)}
		<div class="workflow-argument w-full -translate-x-16" class:hidden={!arg.visible}>
			<DraggableItem hidePointerBorder id={arg.id} data={arg}>
				<div class="group relative flex w-full items-start gap-2">
					<div
						class="flex flex-shrink-0 items-center opacity-0 transition-opacity duration-200 group-hover:opacity-100"
					>
						{@render action(index)}
						<DraggableHandle class="hover:bg-surface3 size-8 rounded-full p-2" />
					</div>
					<div
						class="bg-background dark:bg-surface1 dark:border-surface3 flex w-[calc(100%-5rem)] grow rounded-lg border p-0 transition-all"
					>
						<WorkflowArgument {arg} {onDelete} />
					</div>
				</div></DraggableItem
			>
		</div>
	{/each}
</DraggableList>

{#snippet action(index: number)}
	<button
		class="button-icon min-h-fit min-w-fit p-2"
		onclick={async (e) => {
			// if command is also pressed, prepend the new argument
			if (e.metaKey) {
				args.unshift({
					id: (args.length + 1).toString(),
					name: '',
					displayLabel: '',
					description: '',
					visible: true
				});
			} else {
				args.splice(index + 1, 0, {
					id: (args.length + 1).toString(),
					name: '',
					displayLabel: '',
					description: '',
					visible: true
				});
			}
		}}
		use:tooltip={'Add argument'}
	>
		<Plus class="size-4" />
	</button>
{/snippet}
