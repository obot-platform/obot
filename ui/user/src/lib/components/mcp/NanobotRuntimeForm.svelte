<script lang="ts">
	import type { NanobotRuntimeConfig } from '$lib/services/chat/types';
	import { Plus, Trash2 } from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		config: NanobotRuntimeConfig;
		readonly?: boolean;
		showRequired?: Record<string, boolean>;
		onFieldChange?: (field: string) => void;
	}
	let { config = $bindable(), readonly, showRequired, onFieldChange }: Props = $props();

	// Initialize args array if it doesn't exist
	if (!config.args) {
		config.args = [];
	}

	function addArgument() {
		if (!config.args) {
			config.args = [];
		}
		config.args.push('');
	}

	function removeArgument(index: number) {
		if (config.args) {
			config.args.splice(index, 1);
		}
	}

	function handlePaste(event: ClipboardEvent, index: number) {
		if (readonly || !config.args) return;

		event.preventDefault();
		const pastedText = event.clipboardData?.getData('text');
		if (!pastedText) return;

		const lines = pastedText.split(/[\r\n]+/).filter((line) => line.trim());
		if (lines.length <= 1) {
			config.args[index] = pastedText;
			return;
		}

		// Remove quotes, commas and trim each line
		const cleanedLines = lines.map((line) => {
			let trimmed = line.trim();
			if (trimmed.endsWith(',')) {
				trimmed = trimmed.slice(0, -1).trim();
			}

			if (
				(trimmed.startsWith('"') && trimmed.endsWith('"')) ||
				(trimmed.startsWith("'") && trimmed.endsWith("'"))
			) {
				trimmed = trimmed.slice(1, -1).trim();
			}
			return trimmed;
		});

		config.args[index] = cleanedLines[0];
		for (let j = 1; j < cleanedLines.length; j++) {
			config.args.splice(index + j, 0, cleanedLines[j]);
		}
	}
</script>

<div
	class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
>
	<h4 class="text-sm font-semibold">Nanobot Runtime Configuration</h4>
	<p class="text-xs text-gray-500 dark:text-gray-400">
		Configure the Nanobot runtime with the specified container image and command.
	</p>

	<!-- Image field (required) -->
	<div class="flex items-center gap-4">
		<label
			for="nanobot-image"
			class={twMerge('w-20 text-sm font-light', showRequired?.image && 'error')}>Image</label
		>
		<input
			id="nanobot-image"
			class={twMerge('text-input-filled w-full dark:bg-black', showRequired?.image && 'error')}
			bind:value={config.image}
			disabled={readonly}
			placeholder="e.g. docker.io/myorg/nanobot-server:latest"
			onblur={() => {
				if (config.image) {
					config.image = config.image.trim();
				}
			}}
			oninput={() => {
				onFieldChange?.('image');
			}}
			required
		/>
	</div>

	<!-- Command field (required) -->
	<div class="flex items-center gap-4">
		<label
			for="nanobot-command"
			class={twMerge('w-20 text-sm font-light', showRequired?.command && 'error')}>Command</label
		>
		<input
			id="nanobot-command"
			class={twMerge('text-input-filled w-full dark:bg-black', showRequired?.command && 'error')}
			bind:value={config.command}
			disabled={readonly}
			placeholder="e.g. python server.py"
			onblur={() => {
				if (config.command) {
					config.command = config.command.trim();
				}
			}}
			oninput={() => {
				onFieldChange?.('command');
			}}
			required
		/>
	</div>

	<!-- Arguments field (optional) -->
	{#if config.args}
		<div class="flex gap-4">
			<span class="pt-2.5 text-sm font-light">Arguments</span>
			<div class="flex min-h-10 grow flex-col gap-4">
				{#each config.args as _arg, i (i)}
					<div class="flex items-center gap-2">
						<input
							class="text-input-filled w-full dark:bg-black"
							bind:value={config.args[i]}
							disabled={readonly}
							placeholder="e.g. --config /app/config.json"
							onblur={() => {
								if (config.args && config.args[i]) {
									config.args[i] = config.args[i].trim();
								}
							}}
							onpaste={(e) => handlePaste(e, i)}
						/>
						{#if !readonly}
							<button
								class="icon-button"
								onclick={() => removeArgument(i)}
								use:tooltip={'Remove argument'}
							>
								<Trash2 class="size-4" />
							</button>
						{/if}
					</div>
				{/each}

				{#if !readonly}
					<div class="flex justify-end">
						<button class="button flex items-center gap-1 text-xs" onclick={addArgument}>
							<Plus class="size-4" /> Argument
						</button>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>
