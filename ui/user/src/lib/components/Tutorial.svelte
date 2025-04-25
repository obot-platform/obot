<script lang="ts">
	import { tutorial, type TutorialStep } from '$lib/actions/tutorial.svelte';
	import { onMount } from 'svelte';

	interface Props {
		id: string;
		steps: TutorialStep[];
	}

	const { steps, id }: Props = $props();
	let currentStep = $state(0);

	const {
		start,
		next,
		prev,
		popover: tutorialPopover,
		title: tutorialTitle,
		description: tutorialDescription,
		destroy
	} = tutorial({
		steps,
		onComplete: () => {
			localStorage.setItem(id, 'true');
			destroy();
		},
		onStepChange: (step: number) => {
			currentStep = step;
		}
	});

	onMount(() => {
		if (!localStorage.getItem(id)) {
			start();
		}
	});
</script>

<div use:tutorialPopover class="hidden">
	<div class="relative flex gap-3">
		<img
			src="/user/images/obot-icon-blue.svg"
			alt="obot blue logo"
			class="absolute -top-23 right-0 size-32"
		/>
		<div
			class="dark:bg-surface2 relative z-10 flex max-w-sm flex-col gap-1 rounded-xl bg-white p-4 pb-2"
		>
			<h4 use:tutorialTitle class="text-md font-semibold">Tutorial Title</h4>
			<p use:tutorialDescription class="mb-2 text-sm font-light">Tutorial Description</p>
			<div class="flex w-full items-center justify-between gap-2">
				{#if currentStep !== steps.length - 1}
					<button
						onclick={() => {
							localStorage.setItem(id, 'true');
							destroy();
						}}
						class="button-text pb-2 pl-0 text-xs"
					>
						Skip Tutorial
					</button>
				{/if}
				<div class="flex items-center gap-4">
					{#if currentStep > 0}
						<button onclick={prev} class="button-secondary text-xs">Previous</button>
					{/if}
					<button class="button-primary text-xs" onclick={next}>
						{currentStep === steps.length - 1 ? 'Finish' : 'Next'}
					</button>
				</div>
			</div>
		</div>
	</div>
</div>
