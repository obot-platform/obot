<script lang="ts">
	import { tooltip, type TooltipOptions } from '$lib/actions/tooltip.svelte';
	import type { Placement } from '@floating-ui/dom';
	import { CircleHelpIcon, CircleQuestionMark } from 'lucide-svelte';
	import type { Component, Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		text?: string;
		children?: Snippet;
		class?: string;
		classes?: {
			icon?: string;
		};
		placement?: Placement;
		popoverWidth?: 'sm' | 'md' | 'lg';
		icon?: Component | typeof CircleQuestionMark;
		interactive?: boolean;
	}

	let {
		text,
		children,
		class: klass,
		classes,
		placement,
		popoverWidth = 'md',
		icon: Icon = CircleHelpIcon,
		interactive = false
	}: Props = $props();

	function getPopoverWidth() {
		switch (popoverWidth) {
			case 'sm':
				return 'w-48';
			case 'md':
				return 'w-64';
			case 'lg':
				return 'w-96';
			default:
				return 'w-64';
		}
	}

	const tooltipOpts: TooltipOptions | undefined = $derived.by(() => {
		const layout = [getPopoverWidth(), 'break-normal'] as string[];
		const base = { disablePortal: true, classes: layout, placement, interactive } as const;
		if (children) {
			return { ...base, snippet: children };
		}
		const t = text?.trim() ?? '';
		if (!t) return undefined;
		return { ...base, text: t };
	});
</script>

<div class={twMerge('size-3', klass)} use:tooltip={tooltipOpts}>
	<Icon class={twMerge('text-gray size-3', classes?.icon)} />
</div>
