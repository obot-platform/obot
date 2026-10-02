<script lang="ts">
	import { changeLocale, getLocale, LOCALE_NAMES, locales, m, type Locale } from '$lib/i18n';
	import { Languages } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		class?: string;
	}

	let { class: klass }: Props = $props();

	const current = getLocale();
</script>

<label class={twMerge('dropdown-link cursor-default', klass)}>
	<Languages class="size-4 shrink-0" />
	<span class="grow">{m.language_label()}</span>
	<select
		class="select select-sm w-fit"
		aria-label={m.language_label()}
		value={current}
		onchange={(e) => changeLocale(e.currentTarget.value as Locale)}
	>
		{#each locales as locale (locale)}
			<option value={locale}>{LOCALE_NAMES[locale]}</option>
		{/each}
	</select>
</label>
