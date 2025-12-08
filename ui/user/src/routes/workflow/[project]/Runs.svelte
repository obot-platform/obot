<script lang="ts">
	import { getLayout } from '$lib/context/chatLayout.svelte';
	import { type Project } from '$lib/services';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';
	import { overflowToolTip } from '$lib/actions/overflow';
	import { formatTime } from '$lib/time';

	interface Props {
		project: Project;
		currentThreadID?: string;
		onSelectRun: (run: any) => void;
		selected?: string;
	}

	let { currentThreadID = $bindable(), onSelectRun, selected }: Props = $props();
	const layout = getLayout();
	const runs = $state([
		{
			id: '1',
			created: new Date(Date.now() - 1000 * 60 * 8).toISOString()
		},
		{
			id: '2',
			created: new Date(Date.now() - 1000 * 60 * 60 * 24).toISOString()
		},
		{
			id: '3',
			created: new Date('2025-12-07T12:05:00Z').toISOString()
		},
		{
			id: '4',
			created: new Date('2025-12-07T12:12:00Z').toISOString()
		}
	]);
</script>

<div class="flex flex-col text-xs">
	<div class="flex items-center justify-between">
		<p class="text-md grow font-medium">Runs</p>
	</div>
	<ul class="flex flex-col" transition:fade>
		{#each runs as run, i (run.id)}
			<li class="flex min-h-9 flex-col">
				<div
					class={twMerge(
						'hover:bg-surface3/90 active:bg-surface3/100 group mb-[2px] flex items-center rounded-md font-light transition-colors duration-200',
						selected === run.id && 'bg-surface3/60 font-medium'
					)}
				>
					<div class="flex grow items-center gap-1 truncate pl-1.5">
						<button
							use:overflowToolTip
							class:font-medium={layout.editTaskID === run.id}
							class="grow py-2 pr-2 pl-1 text-left text-xs"
							onclick={async () => {
								onSelectRun(run);
							}}
						>
							{formatTime(run.created)}
						</button>
					</div>
				</div>
			</li>
		{/each}
	</ul>
</div>
