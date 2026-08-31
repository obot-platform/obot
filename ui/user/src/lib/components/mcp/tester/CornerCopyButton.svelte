<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		text?: string;
		label: string;
		offset?: string;
		class?: string;
		children: Snippet;
	}

	let { text, label, offset = '0px', class: klass, children }: Props = $props();

	function scrollbarGutter(node: HTMLElement) {
		let frame = 0;
		const observed = new WeakSet<Element>();

		const update = () => {
			let width = 0;
			for (const element of node.querySelectorAll('.cm-scroller, pre')) {
				if (!observed.has(element)) {
					observer.observe(element);
					observed.add(element);
				}
				width = Math.max(width, (element as HTMLElement).offsetWidth - element.clientWidth);
			}
			node.style.setProperty('--json-scrollbar', `${width}px`);
		};
		const schedule = () => {
			cancelAnimationFrame(frame);
			frame = requestAnimationFrame(update);
		};

		const observer = new ResizeObserver(schedule);
		observer.observe(node);
		schedule();

		return {
			destroy() {
				cancelAnimationFrame(frame);
				observer.disconnect();
			}
		};
	}
</script>

<div
	class={twMerge('relative min-h-10 min-w-0', klass)}
	style:--copy-offset={offset}
	use:scrollbarGutter
>
	{@render children()}
	<CopyButton
		{text}
		noButtonText
		tooltipText={label}
		classes={{
			button:
				'btn btn-ghost btn-square btn-sm absolute top-2 right-[calc(0.5rem+var(--json-scrollbar,0px)+var(--copy-offset,0px))]'
		}}
	/>
</div>
