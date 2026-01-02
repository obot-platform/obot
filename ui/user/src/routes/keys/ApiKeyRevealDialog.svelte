<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import { AlertTriangle, KeyRound } from 'lucide-svelte';

	interface Props {
		keyValue?: string;
		onClose: () => void;
	}

	let { keyValue, onClose }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();

	$effect(() => {
		if (keyValue) {
			dialog?.open();
		}
	});

	function handleClose() {
		onClose();
		dialog?.close();
	}
</script>

{#if keyValue}
	<ResponsiveDialog
		bind:this={dialog}
		onClose={handleClose}
		title="API Key Created"
		class="w-full max-w-lg"
		disableClickOutside
	>
		<div class="flex flex-col gap-6">
			<div
				class="flex items-start gap-3 rounded-lg border border-amber-500 bg-amber-50 p-4 dark:bg-amber-950/30"
			>
				<AlertTriangle class="size-5 flex-shrink-0 text-amber-600 dark:text-amber-500" />
				<div class="flex flex-col gap-1">
					<p class="text-sm font-medium text-amber-800 dark:text-amber-200">Save this key now</p>
					<p class="text-xs text-amber-700 dark:text-amber-300">
						This is the only time you will be able to see this API key. Make sure to copy and store
						it securely. You will not be able to retrieve it later.
					</p>
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<label class="text-sm font-medium">Your API Key</label>
				<div class="flex items-center gap-2">
					<div
						class="dark:bg-surface1 flex flex-1 items-center gap-2 rounded-md border bg-gray-50 px-3 py-2"
					>
						<KeyRound class="size-4 flex-shrink-0 text-gray-500" />
						<code class="flex-1 font-mono text-sm break-all">{keyValue}</code>
					</div>
					<CopyButton text={keyValue} buttonText="Copy" />
				</div>
			</div>
		</div>

		<div class="mt-6 flex justify-end">
			<button class="button-primary" onclick={handleClose}> I've saved my key </button>
		</div>
	</ResponsiveDialog>
{/if}
