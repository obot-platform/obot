<script lang="ts">
	import { assistants } from '$lib/stores';
	import { darkMode } from '$lib/stores';
	import type { Assistant } from '$lib/services';

	interface Props {
		id?: string;
	}

	let { id }: Props = $props();

	let assistant = $derived(
		$assistants.find((a) => {
			if (id) {
				return a.id === id;
			}
			return a.current;
		})
	);

	function icon(a: Assistant | undefined): string {
		if (!a) {
			return '';
		}

		if ($darkMode) {
			return (a.icons.iconDark ? a.icons.iconDark : a.icons.icon) ?? '';
		}
		return a.icons.icon ?? '';
	}
</script>

{#if icon(assistant)}
	<img src={icon(assistant)} alt="assistant icon" class="h-5 w-5 rounded-full" />
{:else}
	<div class="flex h-5 w-5 items-center justify-center rounded-full bg-gray-200 dark:bg-gray">
		{assistant?.name ? assistant.name[0].toUpperCase() : '?'}
	</div>
{/if}
