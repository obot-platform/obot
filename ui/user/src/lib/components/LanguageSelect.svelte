<script lang="ts">
	import {
		changeLocale,
		getLocale,
		LOCALE_NAMES,
		LOCALE_SHORTHAND,
		locales,
		m,
		type Locale
	} from '$lib/i18n';
	import ResponsiveDialog from './ResponsiveDialog.svelte';
	import { Languages } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		class?: string;
		onOpen?: () => void;
	}

	let { class: klass, onOpen }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selected = $state<Locale>(getLocale());

	function portal(node: HTMLElement) {
		document.body.appendChild(node);
		return {
			destroy() {
				node.remove();
			}
		};
	}

	function openDialog() {
		selected = getLocale();
		onOpen?.();
		dialog?.open();
	}
</script>

<button type="button" class={twMerge('dropdown-link justify-between', klass)} onclick={openDialog}>
	<div class="flex items-center gap-2">
		<Languages class="size-4 shrink-0" />
		{m.language_label()}
	</div>
	<span class="badge badge-xs badge-primary mr-2">
		{LOCALE_SHORTHAND[getLocale()]}
	</span>
</button>

<div use:portal>
	<ResponsiveDialog
		bind:this={dialog}
		title={m.language_select_title()}
		class="w-xs h-auto rounded-xl"
	>
		<div class="md:p-0 p-4">
			<fieldset class="flex flex-col gap-2">
				{#each locales as locale (locale)}
					<label
						class={twMerge(
							'cursor-pointer flex items-center justify-between gap-4',
							selected === locale ? 'btn btn-primary rounded-md!' : 'btn rounded-md! px-5!'
						)}
					>
						<span>{LOCALE_NAMES[locale]}</span>
						<input
							type="radio"
							name="language"
							class="radio text-primary radio-sm"
							checked={selected === locale}
							onchange={(e) => {
								e.preventDefault();
								selected = locale;
							}}
						/>
					</label>
				{/each}
			</fieldset>
			<div class="flex justify-end gap-2 pt-4">
				<button class="btn btn-secondary btn-sm" onclick={() => dialog?.close()}>
					{m.common_cancel()}
				</button>
				<button
					class="btn btn-primary btn-sm"
					onclick={() => {
						changeLocale(selected);
						dialog?.close();
					}}
				>
					{m.common_apply()}
				</button>
			</div>
		</div>
	</ResponsiveDialog>
</div>
