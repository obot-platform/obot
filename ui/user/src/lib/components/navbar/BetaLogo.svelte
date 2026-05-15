<script lang="ts">
	import appPreferences from '$lib/stores/appPreferences.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		chat?: boolean;
		enterprise?: boolean;
		class?: string;
	}
	let { chat, enterprise, class: klass }: Props = $props();

	let logos = $derived({
		dark: {
			chat: appPreferences.current.logos?.darkLogoChat,
			enterprise: appPreferences.current.logos?.darkLogoEnterprise,
			default: appPreferences.current.logos?.darkLogoDefault
		},
		light: {
			chat: appPreferences.current.logos?.logoChat,
			enterprise: appPreferences.current.logos?.logoEnterprise,
			default: appPreferences.current.logos?.logoDefault
		}
	});

	const logoPair = $derived(
		chat
			? { light: logos.light.chat, dark: logos.dark.chat }
			: enterprise
				? { light: logos.light.enterprise, dark: logos.dark.enterprise }
				: { light: logos.light.default, dark: logos.dark.default }
	);

	const heightClass = $derived(chat ? 'h-[43px]' : 'h-12');
	const paddingClass = $derived(chat ? 'pl-[1px]' : '');
	const imgClass = $derived(twMerge(heightClass, paddingClass));
</script>

<div class={twMerge('flex shrink-0', klass)}>
	<img src={logoPair.light} class={twMerge(imgClass, 'dark:hidden')} alt="Obot logo" />
	<img src={logoPair.dark} class={twMerge(imgClass, 'hidden dark:block')} alt="Obot logo" />
</div>
