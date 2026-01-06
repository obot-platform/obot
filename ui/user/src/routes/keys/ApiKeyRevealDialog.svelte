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
			<div class="flex items-start gap-3 rounded-lg border p-4">
				<AlertTriangle class="size-5 flex-shrink-0" />
				<div class="flex flex-col gap-1">
					<p class="text-sm font-medium">Save this key now</p>
					<p class="text-xs">
						This is the only time you will be able to see this API key. Make sure to copy and store
						it securely. You will not be able to retrieve it later.
					</p>
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<label class="text-sm font-medium">Your API Key</label>
				<div class="flex items-center gap-2">
					<div class="bg-surface1 flex flex-1 items-center gap-2 rounded-md border px-3 py-2">
						<KeyRound class="text-on-surface1 size-4 flex-shrink-0" />
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
