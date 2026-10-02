<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import VMcpDragDropStage from '$lib/components/vmcps/VMcpDragDropStage.svelte';
	import { m } from '$lib/i18n';
	import { hasSeenTimestamp, markSeenTimestamp } from '$lib/localstate';
	import { profile } from '$lib/stores';
	import setupSplash from '$lib/stores/setupSplash.svelte';
	import { Layers } from '@lucide/svelte';
	import { onMount } from 'svelte';

	interface Props {
		show?: boolean;
		storageKey?: string;
	}

	let { show = true, storageKey = '@obot/seen-vmcp-introduction' }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let dismissed = $state(true);

	onMount(() => {
		dismissed = hasSeenTimestamp(storageKey, profile.current?.created);
	});

	function dismiss() {
		if (dismissed) return;
		dismissed = true;
		markSeenTimestamp(storageKey);
		dialog?.close();
	}

	$effect(() => {
		if (show && !dismissed && !setupSplash.blocking) {
			dialog?.open();
			return;
		}

		dialog?.close();
	});
</script>

<ResponsiveDialog
	bind:this={dialog}
	class="md:max-w-4xl"
	classes={{
		content: 'p-6'
	}}
	hideClose
	disableClickOutside
	onClose={dismiss}
>
	<h2 class="text-2xl font-semibold mb-4">{m.vmcps_intro_getting_started()}</h2>
	<div class="grid md:grid-cols-2 md:items-center">
		<div class="flex flex-col gap-4">
			<p class="leading-relaxed">
				{m.vmcps_intro_lead_prefix()}<b>{m.vmcps_intro_lead_strong()}</b
				>{m.vmcps_intro_lead_suffix()}
			</p>
			<ul class="space-y-2">
				<li class="flex items-start gap-2">
					{@render point()}
					<div class="flex flex-col">
						<b>{m.vmcps_intro_point_control_title()}</b>
						{m.vmcps_intro_point_control_description()}
					</div>
				</li>
				<li class="flex items-start gap-2">
					{@render point()}
					<div class="flex flex-col">
						<b>{m.vmcps_intro_point_secure_title()}</b>
						{m.vmcps_intro_point_secure_description()}
					</div>
				</li>
				<li class="flex items-start gap-2">
					{@render point()}
					<div class="flex flex-col">
						<b>{m.vmcps_intro_point_identity_title()}</b>
						{m.vmcps_intro_point_identity_description()}
					</div>
				</li>
			</ul>
		</div>

		<div
			aria-labelledby="vmcp-introduction-animation-label"
			class="border-l-2 border-primary pl-8 ml-8 h-full flex flex-col justify-center"
		>
			<p
				id="vmcp-introduction-animation-label"
				class="font-mono text-[0.625rem] tracking-[0.14em] uppercase"
			>
				{m.vmcps_drag_and_drop()}
			</p>
			<VMcpDragDropStage class="mt-2" />
			<p class="mt-2 text-xs">
				{m.vmcps_intro_drag_description()}
			</p>
		</div>
	</div>
	<button type="button" class="btn btn-primary w-full mt-8" onclick={dismiss}
		>{m.vmcps_intro_get_started()}</button
	>
</ResponsiveDialog>

{#snippet point()}
	<div class="p-1 rounded-full bg-primary/10 shrink-0">
		<Layers class="text-primary size-4" />
	</div>
{/snippet}
