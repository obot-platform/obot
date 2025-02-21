<script lang="ts">
	import { assistants, context } from '$lib/stores';
	import { darkMode } from '$lib/stores';
	import type { Project } from '$lib/services';
	import { twMerge } from 'tailwind-merge';
	import type { ProjectTemplate } from '$lib/services/index.js';

	interface Props {
		class?: string;
		project?: Project;
		template?: ProjectTemplate;
		id?: string;
	}

	let { project: projectArg, class: klass, id, template }: Props = $props();
	let project = $derived(projectArg || context.project);
	let assistant = $derived(
		assistants.items.find((a) => {
			if (id && id && a.id === id) {
				return true;
			}
			if (project?.assistantID) {
				return a.id === project.assistantID;
			}
			if (template?.assistantID) {
				return a.id === template.assistantID;
			}
			return a.current;
		})
	);
	let icon = $derived.by(getIcon);

	function getLightIcon(): string {
		if (project?.icons?.icon) {
			return project.icons.icon;
		}
		if (template?.icons?.icon) {
			return template.icons.icon;
		}
		if (assistant?.icons?.icon) {
			return assistant.icons.icon;
		}
		return '';
	}

	function getDarkIcon(): string {
		if (project?.icons?.iconDark) {
			return project.icons.iconDark;
		}
		if (assistant?.icons?.iconDark) {
			return assistant.icons.iconDark;
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
	<img src={icon} alt="assistant icon" class={twMerge('h-8 w-8', klass)} />
{:else}
	<div
		class={twMerge(
			'flex h-8 w-8 items-center justify-center rounded-full bg-gray-200 text-on-background dark:bg-gray',
			klass
		)}
	>
		{project?.name
			? project.name[0].toUpperCase()
			: assistant?.name
				? assistant.name[0].toUpperCase()
				: '?'}
	</div>
{/if}
