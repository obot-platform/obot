<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { Eye, EyeOff } from '@lucide/svelte';
	import type { FullAutoFill } from 'svelte/elements';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		name: string;
		value?: string;
		error?: boolean;
		oninput?: () => void;
		onfocus?: () => void;
		textarea?: boolean;
		disabled?: boolean;
		readonly?: boolean;
		growable?: boolean;
		class?: string;
		classes?: {
			wrapper?: string;
			input?: string;
		};
		hideReveal?: boolean;
		placeholder?: string;
		autocomplete?: FullAutoFill;
		minlength?: number;
		required?: boolean;
		onkeydown?: (ev: KeyboardEvent) => void;
		data1pIgnore?: boolean;
	}

	let {
		name,
		value = $bindable(''),
		error,
		oninput,
		onfocus,
		textarea,
		disabled,
		readonly,
		growable,
		class: klass,
		classes,
		hideReveal,
		placeholder,
		autocomplete = 'new-password',
		minlength,
		required,
		onkeydown,
		data1pIgnore = true
	}: Props = $props();

	let showSensitive = $state(false);
	let textareaElement = $state<HTMLTextAreaElement>();
	let scrollableWrapper = $state<HTMLElement>();
	let isResizing = $state(false);
	let startY = $state(0);
	let startHeight = $state(0);
	let manuallyResized = $state(false);
	let textareaScrollTop = $state(0);
	let textareaScrollLeft = $state(0);
	let validationFailed = $state(false);

	$effect(() => {
		void value;
		if (validationFailed && textareaElement?.validity.valid) {
			validationFailed = false;
		}
	});

	$effect(() => {
		void value;
		if (growable && !manuallyResized) {
			resizeGrowableTextarea();
		}
	});

	function getMaskedValue(text: string): string {
		return text.replace(/[^\s]/g, '•').replaceAll(/\n/g, '<br>');
	}

	function handleResizeStart(ev: MouseEvent) {
		ev.preventDefault();
		ev.stopPropagation();

		if (!scrollableWrapper) return;

		isResizing = true;
		manuallyResized = true;
		startY = ev.clientY;
		startHeight = scrollableWrapper.offsetHeight;

		document.addEventListener('mousemove', handleResizeMove);
		document.addEventListener('mouseup', handleResizeEnd);
	}

	function handleResizeMove(ev: MouseEvent) {
		if (!isResizing || !scrollableWrapper) return;

		const deltaY = ev.clientY - startY;
		const newHeight = Math.max(60, startHeight + deltaY); // Min height 60px
		scrollableWrapper.style.height = `${newHeight}px`;
		scrollableWrapper.style.minHeight = `${newHeight}px`;
		scrollableWrapper.style.maxHeight = 'none';
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

	function resizeGrowableTextarea() {
		if (!textareaElement || !scrollableWrapper) return;

		scrollableWrapper.style.height = 'auto';
		textareaElement.style.height = 'auto';
		scrollableWrapper.style.height = `${Math.max(60, textareaElement.scrollHeight)}px`;
		textareaElement.style.height = '100%';
	}

	function handleTextareaScroll(ev: Event) {
		const textarea = ev.currentTarget as HTMLTextAreaElement;
		textareaScrollTop = textarea.scrollTop;
		textareaScrollLeft = textarea.scrollLeft;
	}

	function handleTextareaInvalid() {
		validationFailed = true;
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
	{#if !showSensitive}
		<div class="pointer-events-none absolute inset-0 w-full overflow-hidden">
			<div
				tabindex="-1"
				style:transform={`translate(${-textareaScrollLeft}px, ${-textareaScrollTop}px)`}
				class={twMerge(
					'layer-1 w-full bg-transparent font-mono wrap-break-word whitespace-pre-wrap text-base-content',
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
		<div class="relative flex min-h-15 w-full flex-col leading-5">
			{#if growable}
				<div
					bind:this={scrollableWrapper}
					class={twMerge(
						'text-input-filled base flex min-h-15 w-full shrink-0 flex-col overflow-x-hidden overflow-y-auto font-mono',
						klass,
						classes?.wrapper,
						(error || validationFailed) &&
							'border-error bg-error/20 text-error ring-error focus:ring-1',
						disabled && 'opacity-50',
						!showSensitive ? 'hide' : ''
					)}
				>
					<div class="relative w-full flex-1">
						<textarea
							bind:this={textareaElement}
							class={twMerge(
								'scrollbar-none h-full min-h-15 w-full resize-none bg-transparent outline-none',
								manuallyResized ? 'overflow-auto' : 'overflow-hidden'
							)}
							data-1p-ignore={data1pIgnore}
							id={name}
							{name}
							{disabled}
							{readonly}
							{placeholder}
							{autocomplete}
							{minlength}
							{required}
							spellcheck="false"
							aria-invalid={error || validationFailed || undefined}
							onscroll={handleTextareaScroll}
							oninvalid={handleTextareaInvalid}
							bind:value={
								() => value,
								(v) => {
									if (!readonly) {
										value = v;
										oninput?.();
									}
								}
							}
							onfocus={handleFocus}
							{onkeydown}
						></textarea>

						{@render maskedValue()}
					</div>

					<!-- Resize handle -->
					<div
						class="absolute right-1 bottom-1 z-3 h-3 w-3 cursor-ns-resize select-none"
						onmousedown={handleResizeStart}
						role="button"
						tabindex="-1"
						aria-label="Resize"
					>
						<svg
							class="h-full w-full text-muted-content hover:text-base-content"
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
						classes?.wrapper,
						(error || validationFailed) &&
							'border-error bg-error/20 text-error ring-error focus:ring-1',
						!showSensitive ? 'hide' : ''
					)}
				>
					<div class="relative flex w-full flex-1">
						<textarea
							bind:this={textareaElement}
							class="scrollbar-none h-full w-full flex-1 bg-transparent outline-none"
							data-1p-ignore={data1pIgnore}
							id={name}
							{name}
							{disabled}
							{readonly}
							{placeholder}
							{autocomplete}
							{minlength}
							{required}
							spellcheck="false"
							aria-invalid={error || validationFailed || undefined}
							onscroll={handleTextareaScroll}
							oninvalid={handleTextareaInvalid}
							bind:value={
								() => value,
								(v) => {
									value = v;
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
			data-1p-ignore={data1pIgnore}
			id={name}
			{name}
			class={twMerge(
				'input-text-filled w-full pr-10',
				klass,
				classes?.input,
				error && 'border-error bg-error/20 text-error ring-error focus:ring-1'
			)}
			{value}
			type={showSensitive ? 'text' : 'password'}
			oninput={handleInput}
			onfocus={handleFocus}
			{autocomplete}
			{minlength}
			{required}
			{disabled}
			{readonly}
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
				aria-label={showSensitive ? 'Hide' : 'Reveal'}
				type="button"
				class="cursor-pointer transition-colors duration-150"
				class:text-error={error}
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
	.text-input-filled.base.hide textarea {
		color: transparent;
		caret-color: var(--color-base-content);
	}
	.text-input-filled.base.hide textarea::selection {
		background: highlight;
		color: transparent;
	}
</style>
