<script lang="ts">
	import Toggle from '$lib/components/Toggle.svelte';
	import { MultiValueInput } from '$lib/components/ui/multi-value-input';
	import { m } from '$lib/i18n';
	import type { UVXRuntimeConfig } from '$lib/services/user/types';
	import IconButton from '../primitives/IconButton.svelte';
	import { Plus, Trash2 } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		config: UVXRuntimeConfig;
		startupTimeoutSeconds?: number;
		readonly?: boolean;
		showEgressDomains?: boolean;
		defaultDenyAllEgress?: boolean;
		showRequired?: Record<string, boolean>;
		onFieldChange?: (field: string) => void;
		children?: Snippet;
	}
	let {
		config = $bindable(),
		startupTimeoutSeconds = $bindable(),
		readonly,
		showEgressDomains = false,
		defaultDenyAllEgress = false,
		showRequired,
		onFieldChange,
		children
	}: Props = $props();

	// Initialize args array if it doesn't exist
	if (!config.args) {
		config.args = [];
	}
	if (!config.egressDomains) {
		config.egressDomains = [];
	}

	const hasEgressDomains = $derived(config.egressDomains.some((domain) => domain.trim()));
	const explicitAllowAll = $derived(
		defaultDenyAllEgress && config.denyAllEgress === false && !hasEgressDomains
	);
	const explicitDenyAll = $derived(!defaultDenyAllEgress && config.denyAllEgress === true);
	const toggleChecked = $derived(defaultDenyAllEgress ? explicitAllowAll : explicitDenyAll);
	const toggleLabel = $derived(
		defaultDenyAllEgress ? m.mcps_egress_allow_all() : m.mcps_egress_deny_all()
	);
	const inputReadonly = $derived(readonly || toggleChecked);
	const egressHelpText = $derived(
		defaultDenyAllEgress ? m.mcps_egress_help_default_deny() : m.mcps_egress_help_default_allow()
	);

	function handleEgressToggle(checked: boolean) {
		if (defaultDenyAllEgress) {
			config.denyAllEgress = checked ? false : undefined;
		} else {
			config.denyAllEgress = checked ? true : undefined;
		}
		if (checked) {
			config.egressDomains = [];
		}
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
	class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
>
	<h4 class="text-sm font-semibold">{m.mcps_catalog_uvx_runtime_uvx_title()}</h4>
	<p class="text-muted-content text-xs">{m.mcps_catalog_uvx_runtime_stdio_servers_only()}</p>

	<!-- Package field (required) -->
	<div class="flex items-center gap-4">
		<label
			for="uvx-package"
			class={twMerge('text-sm font-light min-w-[76px]', showRequired?.package && 'error')}
			>{m.mcps_runtime_package()}</label
		>
		<input
			id="uvx-package"
			class={twMerge('text-input-filled dark:bg-base-100 w-full', showRequired?.package && 'error')}
			bind:value={config.package}
			disabled={readonly}
			placeholder={m.mcps_example({ example: 'mcp-server-fetch' })}
			onblur={() => {
				if (config.package) {
					config.package = config.package.trim();
				}
			}}
			oninput={() => {
				onFieldChange?.('package');
			}}
			required
		/>
	</div>

	<!-- Command field (optional) -->
	<div class="flex items-center gap-4">
		<label for="uvx-command" class="text-sm font-light"
			>{m.mcps_catalog_uvx_runtime_command()}</label
		>
		<input
			id="uvx-command"
			class="text-input-filled dark:bg-base-100 w-full"
			bind:value={config.command}
			disabled={readonly}
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
			<span class="pt-2.5 text-sm font-light">{m.mcps_runtime_arguments()}</span>
			<div class="flex min-h-10 grow flex-col gap-4">
				{#each config.args as _arg, i (i)}
					<div class="flex items-center gap-2">
						<input
							class="text-input-filled dark:bg-base-100 w-full"
							bind:value={config.args[i]}
							disabled={readonly}
							placeholder={m.mcps_example({ example: '/path/to/directory' })}
							onblur={() => {
								if (config.args && config.args[i]) {
									config.args[i] = config.args[i].trim();
								}
							}}
							onpaste={(e) => handlePaste(e, i)}
						/>
						{#if !readonly}
							<IconButton
								variant="danger"
								onclick={() => removeArgument(i)}
								tooltip={{ text: m.mcps_runtime_remove_argument() }}
							>
								<Trash2 class="size-4" />
							</IconButton>
						{/if}
					</div>
				{/each}

				{#if !readonly}
					<div class="flex justify-end">
						<button
							type="button"
							class="btn btn-secondary btn-sm flex items-center gap-1"
							onclick={addArgument}
						>
							<Plus class="size-4" />
							{m.mcps_runtime_argument()}
						</button>
					</div>
				{/if}
			</div>
		</div>
	{/if}

	{#if showEgressDomains}
		<div class="flex gap-4">
			<span class="pt-2.5 text-sm font-light">{m.mcps_egress_domains()}</span>
			<div class="flex min-h-10 grow flex-col gap-2">
				<Toggle
					label={toggleLabel}
					labelInline
					checked={toggleChecked}
					disabled={readonly}
					onChange={handleEgressToggle}
				/>
				<MultiValueInput
					bind:value={config.egressDomains}
					class="text-input-filled dark:bg-base-100"
					readonly={inputReadonly}
					placeholder={m.mcps_hit_enter_to_insert()}
				/>
				<p class="text-muted-content text-xs">{egressHelpText}</p>
			</div>
		</div>
	{/if}

	<!-- Startup Timeout -->
	<div class="flex items-center gap-4">
		<label
			for="uvx-startup-timeout"
			class={twMerge('text-sm font-light', showRequired?.startupTimeoutSeconds && 'error')}
			>{m.mcps_runtime_startup_timeout()}</label
		>
		<input
			type="number"
			id="uvx-startup-timeout"
			min="1"
			placeholder="60"
			bind:value={startupTimeoutSeconds}
			class={twMerge(
				'text-input-filled dark:bg-base-100 w-32',
				showRequired?.startupTimeoutSeconds && 'error'
			)}
			disabled={readonly}
		/>
	</div>

	{@render children?.()}
</div>
