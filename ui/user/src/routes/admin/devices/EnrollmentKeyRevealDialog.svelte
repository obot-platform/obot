<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { TriangleAlert, KeyRound } from '@lucide/svelte';

	interface Props {
		credential?: string;
		onClose: () => void;
	}

	let { credential, onClose }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();

	$effect(() => {
		if (credential) {
			dialog?.open();
		}
	});

	function handleClose() {
		onClose();
		dialog?.close();
	}
</script>

{#if credential}
	<ResponsiveDialog
		bind:this={dialog}
		onClose={handleClose}
		title="Enrollment Key Created"
		class="w-full max-w-lg"
		disableClickOutside
	>
		<div class="flex flex-col gap-6">
			<div class="notification-alert">
				<div class="flex items-start gap-3">
					<TriangleAlert class="size-5 shrink-0" />
					<div class="flex flex-col gap-1">
						<p class="text-sm font-medium">Save this key now</p>
						<p class="text-xs">
							This is the only time the enrollment key is shown. Distribute it through your MDM
							configuration; if you lose it, revoke this key and create a new one.
						</p>
					</div>
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<p class="text-sm font-medium">Enrollment Key</p>
				<div class="flex items-center gap-2">
					<div class="bg-base-200 flex flex-1 items-center gap-2 rounded-md border px-3 py-2">
						<KeyRound class="text-muted-content size-4 shrink-0" />
						<code class="flex-1 font-mono text-sm break-all">{credential}</code>
					</div>
					<CopyButton text={credential} buttonText="Copy" />
				</div>
			</div>
		</div>

		<div class="mt-6 flex justify-end">
			<button class="btn btn-primary" onclick={handleClose}> I've saved my key </button>
		</div>
	</ResponsiveDialog>
{/if}
