<script lang="ts">
	import { darkMode } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		class?: string;
		project?: {
			name?: string;
			icons?: {
				icon?: string;
				iconDark?: string;
			};
		};
	}

	let { project, class: klass }: Props = $props();
	let icon = $derived.by(getIcon);

	function getLightIcon(): string {
		if (project?.icons?.icon) {
			return project.icons.icon;
		}
		return '';
	}

	function getDarkIcon(): string {
		if (project?.icons?.iconDark) {
			return project.icons.iconDark;
		}
		return '';
	}

	function getIcon(): string {
		if (darkMode.isDark && getDarkIcon()) {
			return getDarkIcon();
		}
		if (getLightIcon()) {
			return getLightIcon();
		}
		return '';
	}
</script>

{#if icon}
	<img src={icon} alt="assistant icon" class={twMerge('h-8 w-8 rounded-full shadow-md', klass)} />
{:else}
	<div
		class={twMerge(
			'flex h-8 w-8 items-center justify-center rounded-full bg-gray-200 text-on-background dark:bg-gray',
			klass
		)}
	>
		{project?.name ? project.name[0].toUpperCase() : '?'}
	</div>
{/if}
