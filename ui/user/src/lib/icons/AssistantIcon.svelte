<script lang="ts">
	import { assistants, currentAssistant } from '$lib/stores';
	import { darkMode } from '$lib/stores';
	import type { Assistant } from '$lib/services';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id?: string;
		class?: string;
	}

	let { id, class: klass }: Props = $props();

	let assistant = $derived(
		$currentAssistant ?? $assistants.find((a) => {
			if (id) {
				return a.id === id;
			}
			return a.current;
		})
	);

	function getIcon(a: Assistant | undefined): string {
		if (!a) {
			return '';
		}

		if ($darkMode) {
			return (a.icons.iconDark ? a.icons.iconDark : a.icons.icon) ?? '';
		}
		return a.icons.icon ?? '';
	}
	
	function hasDarkIcon(a: Assistant | undefined): boolean {
		if (!a) {
			return false;
		}
		return !!a.icons.iconDark;
	}
</script>

{#if getIcon(assistant)}
	<img src={getIcon(assistant)} alt="assistant icon" class={twMerge('h-5 w-5', !hasDarkIcon(assistant) && 'dark:invert', klass)} />
{:else}
	<div
		class={twMerge(
			'flex h-5 w-5 items-center justify-center rounded-full bg-gray-200 dark:bg-gray',
			klass
		)}
	>
		{assistant?.name ? assistant.name[0].toUpperCase() : '?'}
	</div>
{/if}
