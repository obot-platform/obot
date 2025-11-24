<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import type { CompositeServerToolRow, MCPCatalogEntry, MCPCatalogServer } from '$lib/services';

	interface Props {
		configuringEntry?: MCPCatalogEntry | MCPCatalogServer;
		onClose?: () => void;
		onCancel?: () => void;
		onSuccess?: () => void;
		tools?: CompositeServerToolRow[];
	}

	let { configuringEntry, tools = [], onClose, onCancel, onSuccess }: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let search = $state('');

	let allToolsEnabled = $derived(tools.every((tool) => tool.enabled));

	let visibleTools = $derived(
		tools.filter(
			(tool) =>
				tool.overrideName.toLowerCase().includes(search.toLowerCase()) ||
				tool.overrideDescription?.toLowerCase().includes(search.toLowerCase())
		)
	);

	export function open() {
		dialog?.open();
	}

	export function close() {
		dialog?.close();
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	animate="slide"
	title={`Configure ${configuringEntry?.manifest?.name ?? 'MCP Server'} Tools`}
	class="bg-surface1 pb-0 md:w-2xl"
	onClose={() => onClose?.()}
>
	<p class="text-on-surface1 mb-4 text-xs font-light">
		Toggle what tools are available to users of this composite server. Or modify the name or
		description of a tool; this will override the default name or description provided by the
		server. It may affect the LLM's ability to understand the tool so be careful when adjusting
		these values.
	</p>
	<div class="relative flex flex-col gap-2 overflow-x-hidden px-0.5">
		<div class="flex w-full justify-end">
			<Toggle
				checked={allToolsEnabled}
				onChange={(checked) => {
					tools.forEach((tool) => {
						tool.enabled = checked;
					});
				}}
				label="Enable All Tools"
				labelInline
				classes={{
					label: 'text-sm gap-2'
				}}
				disablePortal
			/>
		</div>
		<Search
			class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
			onChange={(val) => (search = val)}
			placeholder="Search tools..."
		/>
		{#each visibleTools as tool (tool.id)}
			<div
				class="dark:bg-surface2 dark:border-surface3 bg-background flex gap-2 rounded border border-transparent p-2 shadow-sm"
			>
				<div class="flex grow flex-col gap-1">
					<input
						class="text-input-filled flex-1 text-sm"
						bind:value={tool.overrideName}
						placeholder={tool.originalName}
					/>

					<textarea
						class="text-input-filled mt-1 resize-none text-xs"
						bind:value={tool.overrideDescription}
						placeholder="Enter tool description..."
						rows="2"
					></textarea>
				</div>

				<Toggle
					checked={tool.enabled}
					onChange={(checked) => {
						tool.enabled = checked;
					}}
					label="Enable/Disable Tool"
					disablePortal
				/>
			</div>
		{/each}
	</div>
	<div class="bg-surface1 sticky bottom-0 left-0 mt-4 flex w-full justify-end gap-2 p-4">
		<button
			class="button"
			onclick={() => {
				onCancel?.();
				dialog?.close();
			}}>Cancel</button
		>
		<button
			class="button-primary"
			onclick={() => {
				onSuccess?.();
				dialog?.close();
			}}>Confirm</button
		>
	</div>
</ResponsiveDialog>
