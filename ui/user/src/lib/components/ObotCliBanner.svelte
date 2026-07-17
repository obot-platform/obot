<script lang="ts">
	import { resolve } from '$app/paths';
	import Logo from './Logo.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		description: string;
	}

	let { description }: Props = $props();
	let isHovered = $state(false);

	function setHovered(value: boolean) {
		isHovered = value;
	}
</script>

<div class="bg-base-100 group">
	<div
		class={twMerge(
			'bg-primary/25 dark:bg-primary/50 relative overflow-hidden rounded-lg transition-colors duration-200',
			isHovered && 'bg-primary/30 dark:bg-primary/55'
		)}
	>
		<div class="absolute top-1/2 -translate-y-1/2">
			<Logo
				class={twMerge(
					'md:size-36 size-24 -translate-x-1/2 translate-y-0.5 transition-transform duration-200',
					isHovered && 'scale-110 rotate-2'
				)}
			/>
		</div>
		<div
			class="pl-15 md:pl-22 pr-4 py-2 flex flex-col md:flex-row md:justify-between md:items-center gap-1 md:gap-4"
		>
			<div>
				<p class="font-semibold uppercase">Get Started with Obot CLI</p>
				<p class="font-light text-sm">{description}</p>
			</div>
			<a
				href={resolve('/install-cli')}
				onfocus={() => setHovered(true)}
				onmouseenter={() => setHovered(true)}
				onblur={() => setHovered(false)}
				onmouseleave={() => setHovered(false)}
				class="btn btn-primary md:w-fit w-full"
			>
				Go to Download Page
			</a>
		</div>
	</div>
</div>
