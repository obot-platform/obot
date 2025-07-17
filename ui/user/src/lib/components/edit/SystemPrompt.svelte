<script lang="ts">
	import { autoHeight } from '$lib/actions/textarea';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import { type Project } from '$lib/services';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
</script>

<CollapsePane
	classes={{ header: 'pl-3 py-2 text-md', content: 'p-2' }}
	iconSize={5}
	header="System Prompt"
	helpText={HELPER_TEXTS.prompt}
>
	<textarea
		id="project-instructions"
		class="dark:border-surface3 grow resize-none rounded-lg bg-white p-2 text-sm shadow-sm dark:border dark:bg-black"
		rows="12"
		use:autoHeight
		bind:value={project.prompt}
		placeholder="Describe how your project AI should behave, what it should aim to do, and any special instructions it should follow."
	></textarea>
</CollapsePane>
