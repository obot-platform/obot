<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import type { MCP } from '$lib/services';
	import { responsive } from '$lib/stores';
	import { ChevronRight, ChevronsRight, X } from 'lucide-svelte';

	interface Props {
		mcp: MCP;
		onSubmit: () => void;
		readonly?: boolean;
	}
	let { mcp, readonly, onSubmit }: Props = $props();
	let dialog: HTMLDialogElement | undefined = $state();

	export function open() {
		dialog?.showModal();
	}

	export function close() {
		dialog?.close();
	}
</script>

<dialog
	bind:this={dialog}
	use:clickOutside={() => close()}
	class="default-dialog default-scrollbar-thin w-full p-4 sm:max-w-lg md:p-8"
	class:mobile-screen-dialog={responsive.isMobile}
>
	<button class="icon-button absolute top-4 right-4" onclick={() => close()}>
		{#if responsive.isMobile}
			<ChevronRight class="size-6" />
		{:else}
			<X class="size-6" />
		{/if}
	</button>
	<div class="flex h-full flex-col">
		<div class="flex max-w-sm gap-4">
			<img src={mcp.server.icon} alt={mcp.server.name} class="size-12" />
			<div class="flex flex-col justify-center">
				<h3 class="text-lg font-semibold">{mcp.server.name}</h3>
				<!--
                <p class="flex items-center gap-1 font-light">
                    <Star class="size-4" /> 50.5k
                </p>
            -->
			</div>
		</div>
		<div class="flex grow flex-col gap-8 pt-8">
			<p class="md:text-md text-sm font-light text-gray-500">{mcp.server.description}</p>
			{#if readonly}
				{@render readOnlyContent()}
			{:else}
				{@render editContent()}
			{/if}
		</div>
		<button
			onclick={() => onSubmit()}
			class="button-primary mt-8 flex w-full items-center justify-center gap-1 self-end md:w-fit"
		>
			Chat with this server <ChevronsRight class="size-4" />
		</button>
	</div>
</dialog>

{#snippet readOnlyContent()}
	<!-- temporarily commented out, need data from endpoint
    <div class="border-surface2 dark:border-surface3 relative w-full rounded-lg border-2 p-5 pt-2">
        <h4
            class="dark:bg-surface2 absolute top-0 left-3 w-fit -translate-y-3.5 bg-white px-2 text-base font-semibold"
        >
            What You'll Need
        </h4>
        <ul class="mt-4 flex flex-col items-baseline gap-4">
            <li class="flex w-full flex-col gap-1">
                <div class="flex-1 text-sm font-medium capitalize">Foobar API Key</div>
                <div>The API key received from Foobar</div>
            </li>
            <li class="flex w-full flex-col gap-1">
                <div class="flex-1 text-sm font-light capitalize">Biz API Key</div>
                <div>The API key received from Biz.biz API</div>
            </li>
        </ul>
    </div>
-->

	<!--
    <CollapsePane
        header="Tools"
        classes={{
            header: 'px-0',
            content:
                'p-0 pt-4 border-t border-surface2 dark:border-surface3 min-h-[150px] max-h-[30vh] default-scrollbar-thin overflow-y-auto'
        }}
    >
        <div class="flex flex-col gap-2">
            {#each tools as tool (tool.id)}
                <div class="border-surface2 dark:border-surface3 w-[calc(100%-2rem)] border-b pb-4">
                    <p class="text-sm font-medium">{tool.name}</p>
                    <span class="flex text-xs leading-3.5 text-gray-500">{tool.description}</span>
                </div>
            {/each}
        </div>
    </CollapsePane>
-->
{/snippet}

{#snippet editContent()}
	<div class="flex w-full grow flex-col">TODO</div>
{/snippet}
