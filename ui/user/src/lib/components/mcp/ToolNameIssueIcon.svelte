<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { type ToolNameIssue } from '$lib/services/chat/mcp';
	import { TriangleAlert } from 'lucide-svelte';

	interface Props {
		issue: ToolNameIssue | undefined;
		// Render the tooltip inline instead of portaling to document.body.
		// Required when the icon lives inside a native <dialog> modal, whose
		// top layer hides body-portaled tooltips behind the backdrop.
		disablePortal?: boolean;
	}

	let { issue, disablePortal = false }: Props = $props();
</script>

{#if issue}
	<span
		class={`inline-flex shrink-0 items-center ${issue.severity === 'error' ? 'text-error' : 'text-warning'}`}
		use:tooltip={{ text: issue.message, placement: 'top', disablePortal }}
		aria-label={issue.message}
	>
		<TriangleAlert class="size-4 shrink-0" />
	</span>
{/if}
