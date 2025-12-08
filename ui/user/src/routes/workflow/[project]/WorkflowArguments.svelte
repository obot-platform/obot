<script lang="ts">
	import {
		DraggableList,
		DraggableItem,
		DraggableHandle
	} from '$lib/components/primitives/draggable';
	import WorkflowArgument from './WorkflowArgument.svelte';

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
	class="gap-6"
	gap={12}
	order={args.map((item) => item.id)}
	onChange={(items) => {
		args = items as Argument[];
	}}
>
	{#each args as arg (arg.id)}
		<div class="workflow-argument" class:hidden={!arg.visible}>
			<DraggableItem
				id={arg.id}
				data={arg}
				class="hover:border-primary hover:outline-primary bg-background dark:bg-surface1 dark:border-surface3 rounded-lg border p-0 outline-1 outline-transparent transition-all"
			>
				<div class="group relative flex h-full w-full items-start gap-3">
					<DraggableHandle class="absolute top-0 left-0 h-full w-full opacity-0" />
					<WorkflowArgument {arg} {onDelete} />
				</div>
			</DraggableItem>
		</div>
	{/each}
</DraggableList>
