<script lang="ts">
	import type { ChatMessageItemText } from '$lib/services/nanobot/types';
	import { extractPolicyExplanation } from '$lib/services/nanobot/utils';
	import { ShieldAlert } from 'lucide-svelte';

	interface Props {
		item: ChatMessageItemText;
		role: 'user' | 'assistant';
	}

	let { item }: Props = $props();

	const explanation = $derived(extractPolicyExplanation(item.text));
</script>

<div class="border-warning/20 bg-warning/10 mt-3 mb-3 rounded-lg border p-3">
	<div class="mb-2 flex items-center gap-2 text-sm">
		<ShieldAlert class="text-warning h-4 w-4" />
		<span class="text-warning font-medium">Policy Violation</span>
	</div>
	<p class="text-base-content text-sm">{explanation}</p>
</div>
