<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import '@xterm/xterm/css/xterm.css';
	import { currentAssistant } from '$lib/stores';

	let terminalContainer: HTMLElement;
	let close: () => void;

	onDestroy(() => close?.());

	onMount(async () => {
		const { Terminal } = await import('@xterm/xterm');
		const { FitAddon } = await import('@xterm/addon-fit');

		const term = new Terminal();
		const fitAddon = new FitAddon();
		term.loadAddon(fitAddon);

		term.open(terminalContainer);

		new ResizeObserver(() => {
			console.log('div resized');
			fitAddon.fit();
		}).observe(terminalContainer);

		// Example: Connect to a WebSocket server
		const url =
			window.location.protocol.replaceAll('http', 'ws') +
			'//' +
			window.location.host +
			'/api/assistants/' +
			$currentAssistant.id +
			'/shell';
		const socket = new WebSocket(url);
		socket.onmessage = (event) => term.write(event.data);
		socket.onopen = () => {
			term.write('\n');
			term.focus();
		};
		term.onData((data) => socket.send(data));
		term.onResize(({ cols, rows }) => {
			console.log('resize', cols, rows);
			const data = JSON.stringify({ cols, rows });
			socket.send(new Blob([data], { type: 'application/json' }));
		});

		close = () => {
			socket.close();
			term.dispose();
		};
	});
</script>

<div class="h-full w-full rounded-3xl bg-black p-5">
	<div bind:this={terminalContainer}></div>
</div>

<style lang="postcss">
	:global {
		.xterm > div {
			@apply scrollbar-none;
		}
	}
</style>
