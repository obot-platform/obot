<script lang="ts">
	import { profile, term, context, tools } from '$lib/stores';
	import Navbar from '$lib/components/Navbar.svelte';
	import Editor from '$lib/components/Editors.svelte';
	import { EditorService } from '$lib/services';
	import Notifications from '$lib/components/Notifications.svelte';
	import { assistants } from '$lib/stores';
	import Thread from '$lib/components/Thread.svelte';
	import Threads from '$lib/components/Threads.svelte';
	import type { Snippet } from 'svelte';
	import EditMode from '$lib/components/EditMode.svelte';
	import { columnResize } from '$lib/actions/resize';

	let editorVisible = $derived(EditorService.isVisible() || term.open);

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();

	let title = $derived(context.project?.name || assistants.current()?.name || '');
	let mainInput = $state<HTMLDivElement>();

	$effect(() => {
		if (profile.current.unauthorized) {
			// Redirect to the main page to log in.
			window.location.href = `/?rd=${window.location.pathname}`;
		}
	});
</script>

<svelte:head>
	{#if title}
		<title>{title}</title>
	{/if}
</svelte:head>

<div class="h-svh">
	<EditMode>
		<div class="flex h-full">
			{#if tools.hasTool('threads')}
				<Threads />
			{/if}

			<div class="flex h-full grow flex-col">
				<div style="height: 76px">
					<Navbar />
				</div>
				<main
					id="main-content"
					class="flex h-[calc(100vh-76px)] max-w-[calc(100vw-24px)]"
				>
					<div
						bind:this={mainInput}
						id="main-input"
						class="flex h-full {editorVisible ? 'w-2/5' : 'grow'}"
					>
						<Thread id={context.currentThreadID} />
					</div>

					{@render children()}

					{#if editorVisible}
						<div class="w-4 translate-x-4 cursor-col-resize" use:columnResize={mainInput}></div>
						<div
							class="w-3/5 grow rounded-tl-3xl border-4 border-b-0 border-r-0 border-surface2 p-5 transition-all"
						>
							<Editor />
						</div>
					{/if}
				</main>

				<Notifications />
			</div>
		</div>
	</EditMode>
</div>
