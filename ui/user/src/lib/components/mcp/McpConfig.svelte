<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import type { MCP, ProjectMCP } from '$lib/services';
	import { responsive } from '$lib/stores';
	import { ChevronRight, ChevronsRight, CircleCheckBig, X } from 'lucide-svelte';
	import type { Snippet } from 'svelte';

	interface Props {
		mcp: MCP | ProjectMCP;
		disableOutsideClick?: boolean;
		hideCloseButton?: boolean;
		hideSubmitButton?: boolean;
		onSubmit?: (values?: Record<string, string>) => void;
		readonly?: boolean;
		selected?: boolean;
		submitText?: string;
	}
	let {
		mcp,
		disableOutsideClick,
		hideCloseButton,
		hideSubmitButton,
		onSubmit,
		selected,
		submitText,
		readonly
	}: Props = $props();
	let dialog: HTMLDialogElement | undefined = $state();

	export function open() {
		dialog?.showModal();
	}

	export function close() {
		dialog?.close();
	}

	function handleSubmit() {
		const values = {}; // TODO: get values from inputs
		onSubmit?.(readonly ? undefined : values);
		dialog?.close();
	}
</script>

{#if mcp}
	<dialog
		bind:this={dialog}
		use:clickOutside={() => {
			if (disableOutsideClick) return;
			dialog?.close();
		}}
		class="default-dialog default-scrollbar-thin w-full p-4 sm:max-w-lg md:p-8"
		class:mobile-screen-dialog={responsive.isMobile}
	>
		{#if !hideCloseButton}
			<button class="icon-button absolute top-4 right-4" onclick={() => dialog?.close()}>
				{#if responsive.isMobile}
					<ChevronRight class="size-6" />
				{:else}
					<X class="size-6" />
				{/if}
			</button>
		{/if}
		<div class="flex h-full flex-col">
			<div class="flex max-w-sm gap-4">
				<img
					src={'server' in mcp ? mcp.server.icon : mcp.icon}
					alt={'server' in mcp ? mcp.server.name : mcp.name}
					class="size-8"
				/>
				<div class="flex flex-col justify-center">
					<h3 class="text-lg font-semibold">{'server' in mcp ? mcp.server.name : mcp.name}</h3>
					<!--
                <p class="flex items-center gap-1 font-light">
                    <Star class="size-4" /> 50.5k
                </p>
            -->
				</div>
			</div>
			<div class="flex grow flex-col gap-8 pt-4">
				<p class="md:text-md text-sm font-light text-gray-500">
					{'server' in mcp ? mcp.server.description : mcp.description}
				</p>
				{#if readonly}
					{@render readOnlyContent()}
				{:else}
					{@render editContent()}
				{/if}
			</div>
			{#if selected}
				<div class="mt-8 flex items-center gap-2 self-end">
					<p class="text-md text-gray-500">Server added to Obot</p>
					<CircleCheckBig class="size-5 text-blue-500" />
				</div>
			{:else if !hideSubmitButton}
				<button
					onclick={handleSubmit}
					class="button-primary mt-8 flex w-full items-center justify-center gap-1 self-end md:w-fit"
				>
					{submitText ?? 'Chat with this server'}
					<ChevronsRight class="size-4" />
				</button>
			{/if}
		</div>
	</dialog>
{/if}

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
	<!-- <div class="border-surface2 dark:border-surface3 relative w-full rounded-lg border-2 p-5 pt-2">
		<h4
			class="dark:bg-surface2 absolute top-0 left-3 w-fit -translate-y-3.5 bg-white px-2 text-base font-semibold"
		>
			Set Up Your MCP
		</h4>
		<div class="mt-4 flex flex-col items-baseline gap-4">
			<div class="flex w-full flex-col gap-1">
				<label for="from" class="text-sm font-light">Foobar API Key</label>
				<input id="from" class="text-input-filled" placeholder="" />
			</div>
			<div class="flex w-full flex-col gap-1">
				<label for="from" class="text-sm font-light">Biz API Key</label>
				<input id="from" class="text-input-filled" placeholder="" />
			</div>
		</div>
	</div> -->
{/snippet}
