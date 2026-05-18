<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import type { ImagePullSecret, ImagePullSecretTestResponse } from '$lib/services';
	import FieldLabel from './FieldLabel.svelte';
	import { displayName } from './types';
	import { CircleAlert, CircleCheck, LoaderCircle, ShieldCheck } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		secret?: ImagePullSecret;
		testImage: string;
		testing?: boolean;
		testResult?: ImagePullSecretTestResponse;
		testError?: string;
		onTest: () => void;
		onClose?: () => void;
	}

	let {
		secret,
		testImage = $bindable(''),
		testing = false,
		testResult,
		testError = '',
		onTest,
		onClose
	}: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();

	export function open() {
		dialog?.open();
	}

	export function close() {
		dialog?.close();
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	title={`Test ${secret ? displayName(secret) : 'Image Pull Secret'}`}
	class="w-full md:max-w-xl"
	{onClose}
>
	<form
		class="flex flex-col gap-4"
		onsubmit={(e) => {
			e.preventDefault();
			onTest();
		}}
	>
		<label class="flex flex-col gap-1">
			<FieldLabel
				label="Test Image Reference"
				help="Full image reference to pull with this image pull secret during the connection test."
			/>
			<input
				class="input-text-filled"
				bind:value={testImage}
				disabled={testing}
				placeholder={secret?.manifest.type === 'ecr'
					? '123456789012.dkr.ecr.us-east-1.amazonaws.com/app:latest'
					: 'registry.example.com/team/app:latest'}
				required
			/>
		</label>

		{#if testResult || testError}
			<div
				class={twMerge(
					'flex items-center gap-3 rounded-md border p-3 text-sm',
					testError
						? 'border-red-500 bg-red-500/10 text-red-700 dark:text-red-300'
						: 'border-green-500 bg-green-500/10 text-green-700 dark:text-green-300'
				)}
			>
				{#if testError}
					<CircleAlert class="size-5 shrink-0" />
					<span>{testError}</span>
				{:else}
					<CircleCheck class="size-5 shrink-0" />
					<span>{testResult?.message || 'Success'}</span>
				{/if}
			</div>
		{/if}

		<div class="flex justify-end gap-2">
			<button type="button" class="button" disabled={testing} onclick={close}>Cancel</button>
			<button
				type="submit"
				class="button-primary flex items-center gap-1 text-sm"
				disabled={testing || !testImage.trim()}
			>
				{#if testing}
					<LoaderCircle class="size-4 animate-spin" />
				{:else}
					<ShieldCheck class="size-4" />
				{/if}
				Test
			</button>
		</div>
	</form>
</ResponsiveDialog>
