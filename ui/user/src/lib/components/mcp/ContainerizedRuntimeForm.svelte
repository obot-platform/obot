<script lang="ts">
	import type { ContainerizedRuntimeConfig } from '$lib/services/chat/types';
	import { Plus, Trash2 } from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		config: ContainerizedRuntimeConfig;
		readonly?: boolean;
	}
	let { config = $bindable(), readonly }: Props = $props();

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

	function handlePortInput(event: Event) {
		const target = event.target as HTMLInputElement;
		const value = target.value.trim();

		// Allow empty value for intermediate states
		if (value === '') {
			config.port = 0;
			return;
		}

		const port = parseInt(value, 10);
		if (!isNaN(port) && port > 0 && port <= 65535) {
			config.port = port;
		} else {
			// Reset to previous valid value or default
			target.value = config.port > 0 ? config.port.toString() : '';
		}
	}
</script>

<div
	class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
>
	<h4 class="text-sm font-semibold">Containerized Runtime Configuration</h4>
	<p class="text-xs text-gray-500 dark:text-gray-400">
		Only Streamable HTTP and SSE servers are supported.
	</p>

	<!-- Image field (required) -->
	<div class="flex items-center gap-4">
		<label for="containerized-image" class="text-sm font-light">Image</label>
		<input
			id="containerized-image"
			class="text-input-filled w-full dark:bg-black"
			bind:value={config.image}
			disabled={readonly}
			placeholder="e.g. docker.io/myorg/mcp-server:latest"
			onblur={() => {
				if (config.image) {
					config.image = config.image.trim();
				}
			}}
			required
		/>
	</div>

	<!-- Port field (required) -->
	<div class="flex items-center gap-4">
		<label for="containerized-port" class="text-sm font-light">Port</label>
		<input
			id="containerized-port"
			type="number"
			class="text-input-filled w-full dark:bg-black"
			value={config.port > 0 ? config.port : ''}
			disabled={readonly}
			placeholder="e.g. 8080"
			min="1"
			max="65535"
			required
			oninput={handlePortInput}
		/>
	</div>

	<!-- Path field (required) -->
	<div class="flex items-center gap-4">
		<label for="containerized-path" class="text-sm font-light">Path</label>
		<input
			id="containerized-path"
			class="text-input-filled w-full dark:bg-black"
			bind:value={config.path}
			disabled={readonly}
			placeholder="e.g. /mcp"
			onblur={() => {
				if (config.path) {
					config.path = config.path.trim();
				}
			}}
			required
		/>
	</div>

	<!-- Command field (optional) -->
	<div class="flex items-center gap-4">
		<label for="containerized-command" class="text-sm font-light">Command</label>
		<input
			id="containerized-command"
			class="text-input-filled w-full dark:bg-black"
			bind:value={config.command}
			disabled={readonly}
			placeholder="e.g. node server.js"
			onblur={() => {
				if (config.command) {
					config.command = config.command.trim();
				}
			}}
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
