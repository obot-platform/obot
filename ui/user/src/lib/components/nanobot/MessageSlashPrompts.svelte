<script lang="ts">
	import type { Prompt } from '$lib/services/nanobot/types';

	interface Props {
		prompts: Prompt[];
		onPrompt?: (promptName: string) => void;
		message: string;
	}

	let { prompts, onPrompt, message }: Props = $props();

	// Slash command state
	let showSlashCommands = $derived(message.trim().startsWith('/'));
	let filteredPrompts = $derived.by(() => {
		if (!showSlashCommands) return [];
		const query = message.trim().slice(1).toLowerCase();
		return prompts.filter(
			(prompt) =>
				prompt.name.toLowerCase().includes(query) ||
				prompt.title?.toLowerCase().includes(query) ||
				prompt.description?.toLowerCase().includes(query)
		);
	});
	let selectedCommandIndex = $state(0);
	let slashQuery = $derived(message.trim().slice(1).toLowerCase());

	export function handleKeydown(e: KeyboardEvent): boolean {
		// Handle slash command navigation
		if (showSlashCommands) {
			switch (e.key) {
				case 'ArrowDown':
					e.preventDefault();
					selectedCommandIndex = Math.min(selectedCommandIndex + 1, filteredPrompts.length - 1);
					return true;
				case 'ArrowUp':
					e.preventDefault();
					selectedCommandIndex = Math.max(selectedCommandIndex - 1, 0);
					return true;
				case 'Enter':
					e.preventDefault();
					if (filteredPrompts[selectedCommandIndex]) {
						executeSlashCommand(filteredPrompts[selectedCommandIndex]);
					}
					return true;
			}
		}
		return false;
	}

	function executeSlashCommand(prompt: Prompt) {
		onPrompt?.(prompt.name);
	}
</script>

{#if showSlashCommands}
	<div
		class="border-base-300 bg-base-100 z-50 max-h-60 w-full overflow-y-auto rounded-lg border shadow-lg"
		style="top: calc(100% + 0.5rem);"
	>
		{#each filteredPrompts as prompt, index (prompt.name)}
			<button
				type="button"
				class="hover:bg-base-200 w-full px-4 py-2 text-left transition-colors {index ===
				selectedCommandIndex
					? 'bg-primary/10'
					: ''}"
				onclick={() => executeSlashCommand(prompt)}
			>
				<div class="flex items-center space-x-2">
					<span class="text-primary font-mono text-sm">/{prompt.name}</span>
					{#if prompt.title && prompt.title !== prompt.name}
						<span class="text-sm font-medium">{prompt.title}</span>
					{/if}
				</div>
				{#if prompt.description}
					<div class="text-base-content/60 mt-1 text-xs">
						{prompt.description}
					</div>
				{/if}
			</button>
		{/each}

		{#if filteredPrompts.length === 0}
			<div class="text-base-content/50 px-4 py-2 text-sm">
				No commands found for "{slashQuery}"
			</div>
		{/if}
	</div>
{/if}
