<script lang="ts">
	import { darkMode } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';
	import { LOGO_CONFIG } from '$lib/config/logos';

	interface Props {
		chat?: boolean;
		enterprise?: boolean;
		class?: string;
	}
	let { chat, enterprise, class: klass }: Props = $props();

	const logoSrc = $derived.by(() => {
		const theme = darkMode.isDark ? 'dark' : 'light';
		if (chat) {
			return LOGO_CONFIG.beta[theme].chat;
		} else if (enterprise) {
			return LOGO_CONFIG.beta[theme].enterprise;
		}
		return LOGO_CONFIG.beta[theme].default;
	});

	const heightClass = $derived(chat ? 'h-[43px]' : 'h-12');
	const paddingClass = $derived(chat ? 'pl-[1px]' : '');
</script>

<div class={twMerge('flex flex-shrink-0', klass)}>
	<img src={logoSrc} class={twMerge(heightClass, paddingClass)} alt="Obot logo" />
</div>
