<script lang="ts">
	import CopyField from '$lib/components/CopyField.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
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
		title={m.inventory_enforcement_configuration_key_created_title()}
		class="w-full max-w-lg"
		disableClickOutside
	>
		<div class="flex flex-col gap-6">
			<div class="notification-alert">
				<div class="flex items-start gap-3">
					<TriangleAlert class="size-5 shrink-0" />
					<div class="flex flex-col gap-1">
						<p class="text-sm font-medium">
							{m.inventory_enforcement_configuration_save_key_now()}
						</p>
						<p class="text-xs">
							{m.inventory_enforcement_configuration_key_shown_once()}
						</p>
					</div>
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<p class="text-sm font-medium">{m.inventory_enforcement_configuration_enrollment_key()}</p>
				<CopyField value={credential} id="enrollment-key">
					{#snippet preContent()}
						<KeyRound class="text-muted-content size-4 shrink-0" />
					{/snippet}
				</CopyField>
			</div>
		</div>

		<div class="mt-6 flex justify-end">
			<button class="btn btn-primary" onclick={handleClose}>
				{m.inventory_enforcement_configuration_saved_my_key()}
			</button>
		</div>
	</ResponsiveDialog>
{/if}
