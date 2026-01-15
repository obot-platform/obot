<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { Eye, EyeOff } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		name: string;
		value?: string;
		error?: boolean;
		oninput?: () => void;
		onfocus?: () => void;
		textarea?: boolean;
		disabled?: boolean;
		growable?: boolean;
		class?: string;
		hideReveal?: boolean;
		placeholder?: string;
		onkeydown?: (ev: KeyboardEvent) => void;
	}

	let {
		name,
		value = $bindable(''),
		error,
		oninput,
		onfocus,
		textarea,
		disabled,
		growable,
		class: klass,
		hideReveal,
		placeholder,
		onkeydown
	}: Props = $props();

	let showSensitive = $state(false);
	let textareaElement = $state<HTMLElement>();
	let maskedTextarea = $state<HTMLElement>();
	let containerElement = $state<HTMLElement>();
	let scrollableWrapper = $state<HTMLElement>();
	let resizeHandle = $state<HTMLElement>();
	let isResizing = $state(false);
	let startY = $state(0);
	let startHeight = $state(0);

	function getMaskedValue(text: string): string {
		return text.replace(/[^\s]/g, '•').replaceAll(/\n/g, '<br>');
	}

	function handleResizeStart(ev: MouseEvent) {
		ev.preventDefault();
		ev.stopPropagation();

		if (!scrollableWrapper) return;

		isResizing = true;
		startY = ev.clientY;
		startHeight = scrollableWrapper.offsetHeight;

		document.addEventListener('mousemove', handleResizeMove);
		document.addEventListener('mouseup', handleResizeEnd);
	}

	function handleResizeMove(ev: MouseEvent) {
		if (!isResizing || !scrollableWrapper) return;

		const deltaY = ev.clientY - startY;
		const newHeight = Math.max(60, startHeight + deltaY); // Min height 60px
		scrollableWrapper.style.maxHeight = `${newHeight}px`;
		scrollableWrapper.style.minHeight = 'auto';
	}

	function handleResizeEnd() {
		isResizing = false;
		document.removeEventListener('mousemove', handleResizeMove);
		document.removeEventListener('mouseup', handleResizeEnd);
	}

	function handleInput(ev: Event) {
		const input = ev.target as HTMLInputElement;
		value = input.value;
		oninput?.();
	}

	function handleFocus(_: FocusEvent) {
		onfocus?.();
	}

	function toggleVisibility(ev: MouseEvent) {
		ev.preventDefault();
		showSensitive = !showSensitive;

		if (showSensitive) {
			textareaElement?.focus();
		}
	}
</script>

{#snippet maskedValue()}
	{#if !showSensitive && growable}
		<!-- Masked overlay for growable contenteditable -->
		<div class="pointer-events-none absolute inset-0 w-full">
			<div
				bind:this={maskedTextarea}
				tabindex="-1"
				class={twMerge(
					'layer-1 black:text-white w-full bg-transparent font-mono break-words whitespace-pre-wrap text-black',
					klass
				)}
			>
				{@html getMaskedValue(value)}
			</div>
		</div>
	{:else if !showSensitive}
		<!-- Masked overlay for non-growable textarea -->
		<div class="pointer-events-none absolute inset-0 w-full overflow-auto">
			<div
				bind:this={maskedTextarea}
				tabindex="-1"
				class={twMerge(
					'layer-1 black:text-white w-full bg-transparent font-mono break-words whitespace-pre-wrap text-black',
					klass
				)}
			>
				{@html getMaskedValue(value)}
			</div>
		</div>
	{/if}
{/snippet}

<div class="relative flex grow items-center">
	{#if textarea}
		<div bind:this={containerElement} class="relative flex min-h-[60px] w-full flex-col leading-5">
			{#if growable}
				<input type="text" {name} {disabled} {value} {placeholder} hidden />
				<div
					bind:this={scrollableWrapper}
					class={twMerge(
						'text-input-filled base flex w-full flex-1 flex-col overflow-x-hidden overflow-y-auto font-mono',
						klass,
						error && 'border-red-500 bg-red-500/20 text-red-500 ring-red-500 focus:ring-1',
						disabled && 'opacity-50',
						!showSensitive ? 'hide' : ''
					)}
				>
					<div class="relative w-full flex-1">
						<div
							bind:this={textareaElement}
							class="w-full outline-none"
							data-1p-ignore
							id={name}
							contenteditable="plaintext-only"
							spellcheck="false"
							role="textbox"
							tabindex="0"
							onscroll={(ev) => {
								if (!showSensitive && maskedTextarea) {
									maskedTextarea.scrollTop = ev.currentTarget.scrollTop;
									maskedTextarea.scrollLeft = ev.currentTarget.scrollLeft;
								}
							}}
							bind:innerText={
								() => value,
								(v) => {
									value = v.trim();
									oninput?.();
								}
							}
							onfocus={handleFocus}
							{onkeydown}
						></div>

						{@render maskedValue()}

						{#if placeholder && value.length === 0}
							<div
								class="black:text-white/50 pointer-events-none absolute inset-0 z-2 bg-transparent text-black/50"
							>
								{placeholder}
							</div>
						{/if}
					</div>

					<!-- Resize handle -->
					<div
						bind:this={resizeHandle}
						class="absolute right-1 bottom-1 z-3 h-3 w-3 cursor-ns-resize select-none"
						onmousedown={handleResizeStart}
						role="button"
						tabindex="-1"
						aria-label="Resize"
					>
						<svg
							class="h-full w-full text-gray-500 hover:text-gray-700"
							viewBox="0 0 12 12"
							fill="none"
							stroke="currentColor"
							stroke-width="1.5"
						>
							<line x1="0" y1="12" x2="12" y2="0" />
							<line x1="4" y1="12" x2="12" y2="4" />
							<line x1="8" y1="12" x2="12" y2="8" />
						</svg>
					</div>
				</div>
			{:else}
				<div
					class={twMerge(
						'text-input-filled base flex min-h-full w-full flex-1 flex-col overflow-hidden rounded font-mono [box-shadow:none]',
						klass,
						error && 'border-red-500 bg-red-500/20 text-red-500 ring-red-500 focus:ring-1',
						!showSensitive ? 'hide' : ''
					)}
				>
					<div class="relative flex w-full flex-1">
						<textarea
							bind:this={textareaElement}
							class="scrollbar-none h-full w-full flex-1 bg-transparent outline-none"
							data-1p-ignore
							id={name}
							{name}
							{disabled}
							{placeholder}
							spellcheck="false"
							onscroll={(ev) => {
								if (!showSensitive && maskedTextarea) {
									maskedTextarea.parentElement!.scrollTop = ev.currentTarget.scrollTop;
									maskedTextarea.parentElement!.scrollLeft = ev.currentTarget.scrollLeft;
								}
							}}
							bind:value={
								() => value,
								(v) => {
									value = v.trim();
									oninput?.();
								}
							}
							onfocus={handleFocus}
							{onkeydown}
						></textarea>

						{@render maskedValue()}
					</div>
				</div>
			{/if}
		</div>
	{:else}
		<input
			data-1p-ignore
			id={name}
			{name}
			class={twMerge(
				'text-input-filled w-full pr-10',
				klass,
				error &&
					'border-red-500 bg-red-500/20 [color:var(--color-red-500)] ring-red-500 focus:ring-1'
			)}
			{value}
			type={showSensitive ? 'text' : 'password'}
			oninput={handleInput}
			onfocus={handleFocus}
			autocomplete="new-password"
			{disabled}
			{placeholder}
			{onkeydown}
		/>
	{/if}

	{#if !hideReveal}
		<div
			class="absolute top-1/2 right-4 z-10 grid -translate-y-1/2 grid-cols-1 grid-rows-1"
			use:tooltip={{ disablePortal: true, text: showSensitive ? 'Hide' : 'Reveal' }}
		>
			<button
				type="button"
				class="cursor-pointer transition-colors duration-150"
				class:text-red-500={error}
				onclick={toggleVisibility}
			>
				{#if showSensitive}
					<EyeOff class="size-4" />
				{:else}
					<Eye class="size-4" />
				{/if}
			</button>
		</div>
	{/if}
</div>

<style>
	.text-input-filled.base.hide textarea,
	.text-input-filled.base.hide [contenteditable] {
		color: transparent;
		caret-color: var(--color-on-background);
	}
	.text-input-filled.base.hide::selection {
		background: highlight;
		color: transparent;
	}
</style>
