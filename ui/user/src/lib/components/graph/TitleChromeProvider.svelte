<script lang="ts">
	import { TITLE_CHROME, type TitleChrome } from './titleChrome';
	import { setContext, type Snippet } from 'svelte';

	type Props = TitleChrome & {
		children: Snippet;
	};

	let { leading, trailing, children }: Props = $props();

	setContext<TitleChrome>(TITLE_CHROME, {
		get leading() {
			return leading;
		},
		get trailing() {
			return trailing;
		}
	});
</script>

{@render children()}
