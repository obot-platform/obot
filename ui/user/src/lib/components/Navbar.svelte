<script lang="ts">
	import Profile from '$lib/components/navbar/Profile.svelte';
	import { fade } from 'svelte/transition';
	import Logo from '$lib/components/navbar/Logo.svelte';
	import type { Snippet } from 'svelte';
	import EditorToggle from './navbar/EditorToggle.svelte';
	import type { Project } from '$lib/services';
	import { getLayout } from '$lib/context/layout.svelte';

	interface Props {
		children?: Snippet;
		showEditorButton?: boolean;
		project?: Project;
	}

	let { children, showEditorButton, project }: Props = $props();
	const layout = getLayout();
</script>

<nav class="w-full via-80%" in:fade|global>
	<div class="bg-white p-3 dark:bg-black">
		<div class="flex items-center justify-between">
			{#if children}
				{@render children()}
			{:else}
				<Logo />
			{/if}
			<div class="grow"></div>
			{#if showEditorButton && project}
				<EditorToggle />
			{/if}
			{#if !layout.projectEditorOpen}
				<div>
					<Profile />
				</div>
			{/if}
		</div>
	</div>
</nav>
