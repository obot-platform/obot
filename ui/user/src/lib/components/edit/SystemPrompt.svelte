<script lang="ts">
	import { autoHeight } from '$lib/actions/textarea';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { type Project } from '$lib/services';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
</script>

<CollapsePane classes={{ header: 'pl-3 py-2 text-md', content: 'p-2' }} iconSize={5}>
	{#snippet header()}
		<span class="flex grow items-center gap-2 text-start text-sm font-extralight">
			System Prompt
		</span>
	{/snippet}
	<textarea
		id="project-instructions"
		class="dark:border-surface3 grow resize-none rounded-lg bg-white p-2 text-sm shadow-sm dark:border dark:bg-black"
		rows="12"
		use:autoHeight
		bind:value={project.prompt}
		placeholder="Describe your agent's personality, goals, and any other relevant information."
	></textarea>
</CollapsePane>
