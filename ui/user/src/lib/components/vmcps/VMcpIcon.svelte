<script lang="ts">
	import { Layers, Server } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	const MAX_ICON_SLICES = 4;

	interface ComponentIcon {
		name: string;
		icon?: string;
	}

	interface Props {
		components: ComponentIcon[];
		class?: string;
	}

	let { components, class: clazz }: Props = $props();

	let slices = $derived(iconSlices(components));
	let hidden = $derived(components.length - slices.length);

	/**
	 * Lays the first few server icons out as one square: each slice clips a full-size icon down to
	 * the piece of the square it occupies, so the icons read as a single cut-up tile.
	 */
	function iconSlices(componentServers: ComponentIcon[]) {
		if (componentServers.length === 1) {
			return [
				{
					component: componentServers[0],
					cell: 'inset-0',
					icon: 'inset-0 size-full p-1',
					glyph: 'size-5'
				}
			];
		}
		if (componentServers.length === 2) {
			return [
				{
					component: componentServers[0],
					cell: 'top-0 left-0 h-full w-1/2 border-r',
					icon: 'top-0 left-0 h-full w-[200%]',
					glyph: 'size-4'
				},
				{
					component: componentServers[1],
					cell: 'top-0 right-0 h-full w-1/2',
					icon: 'top-0 right-0 h-full w-[200%]',
					glyph: 'size-4'
				}
			];
		}
		const quadrants = [
			{ cell: 'top-0 left-0 border-r border-b', icon: 'top-0 left-0' },
			{ cell: 'top-0 right-0 border-b', icon: 'top-0 right-0' },
			{ cell: 'bottom-0 left-0 border-r', icon: 'bottom-0 left-0' },
			{ cell: 'bottom-0 right-0', icon: 'bottom-0 right-0' }
		];
		return componentServers.slice(0, MAX_ICON_SLICES).map((component, index) => ({
			component,
			cell: `h-1/2 w-1/2 ${quadrants[index].cell}`,
			icon: `h-[200%] w-[200%] ${quadrants[index].icon}`,
			glyph: 'size-3'
		}));
	}
</script>

<div class={twMerge('relative size-10 shrink-0', clazz)}>
	<div class="bg-primary/10 text-primary absolute inset-0 overflow-hidden rounded-md">
		{#if slices.length === 0}
			<div class="flex size-full items-center justify-center">
				<Layers class="size-5" />
			</div>
		{:else}
			{#each slices as slice, sliceIndex (sliceIndex)}
				<div
					class={`border-base-100 dark:border-base-300 absolute overflow-hidden ${slice.cell}`}
					title={slice.component.name}
				>
					{#if slice.component.icon}
						<img
							src={slice.component.icon}
							alt=""
							class={`absolute max-w-none object-contain ${slice.icon}`}
							loading="lazy"
							decoding="async"
						/>
					{:else}
						<div class="flex size-full items-center justify-center">
							<Server class={slice.glyph} />
						</div>
					{/if}
				</div>
			{/each}
		{/if}
	</div>
	{#if hidden > 0}
		<span
			class="bg-primary text-primary-content ring-base-100 dark:ring-base-300 absolute -right-1 -bottom-1 rounded-full px-1 font-mono text-[10px] leading-4 ring-1"
		>
			+{hidden}
		</span>
	{/if}
</div>
