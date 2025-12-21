<script lang="ts">
	import { type Task } from '$lib/services';

	interface Props {
		input?: string;
		displayRunID?: string;
		task?: Task;
	}

	let { input = $bindable(''), task }: Props = $props();

	let params: Record<string, string> = $state({});

	$effect(() => {
		if (task?.onDemand?.params) {
			input = JSON.stringify(params);
		} else {
			input = '';
		}
	});
</script>

<div class="border-surface2 dark:border-surface3 relative w-full rounded-lg border-2 p-5 pt-2">
	<h4
		class="dark:bg-surface2 bg-background absolute top-0 left-3 w-fit -translate-y-3.5 px-2 text-base font-semibold"
	>
		{#if task?.onDemand?.params}
			Arguments
		{/if}
	</h4>

	{#if task?.onDemand?.params}
		<div class="mt-4 flex flex-col items-baseline gap-4">
			{#each Object.keys(task.onDemand.params) as key (key)}
				<div class="flex w-full flex-col gap-1">
					<label for="param-{key}" class="flex-1 text-sm font-medium capitalize">{key}</label>
					<input
						id="param-{key}"
						bind:value={params[key]}
						class="text-input-filled"
						placeholder={task.onDemand.params[key]}
					/>
				</div>
			{/each}
		</div>
	{/if}
</div>
