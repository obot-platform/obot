<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import '@xterm/xterm/css/xterm.css';
	import { currentAssistant } from '$lib/stores';
	import { RefreshCcw } from 'lucide-svelte';
	import { term } from '$lib/stores';
	import Env from '$lib/components/terminal/Env.svelte';

	let terminalContainer: HTMLElement;
	let close: () => void;
	let connectState = $state('disconnected');
	let envDialog: ReturnType<typeof Env>;

	onDestroy(() => close?.());

	onMount(connect);

	function closeTerm() {
		term.open = false;
	}

	async function connect() {
		const { Terminal } = await import('@xterm/xterm');
		const { FitAddon } = await import('@xterm/addon-fit');

		close?.();

		const term = new Terminal();
		const fitAddon = new FitAddon();
		term.loadAddon(fitAddon);

		term.open(terminalContainer);

		new ResizeObserver(() => {
			fitAddon.fit();
		}).observe(terminalContainer);

		const url =
			window.location.protocol.replaceAll('http', 'ws') +
			'//' +
			window.location.host +
			'/api/assistants/' +
			$currentAssistant.id +
			'/shell';
		let gotData = false;
		const socket = new WebSocket(url);
		connectState = 'connecting';
		socket.onmessage = (event) => term.write(event.data);
		socket.onopen = () => {
			connectState = 'connected';
			fitAddon.fit();
			term.focus();
			setTimeout(() => {
				if (!gotData) {
					socket.send('\n');
				}
			}, 500);
		};
		socket.onclose = () => {
			connectState = 'disconnected';
			term.write('\r\nConnection closed.\r\n');
		};
		socket.onerror = () => {
			connectState = 'disconnected';
			term.write('\r\nConnection error.\r\n');
		};
		term.options.theme = {
			background: '#131313'
		};
		term.onData((data) => {
			gotData = true;
			socket.send(data);
		});
		term.onResize(({ cols, rows }) => {
			const data = JSON.stringify({ cols, rows });
			socket.send(new Blob([data], { type: 'application/json' }));
		});

		close = () => {
			socket.close();
			term.dispose();
		};
	}
</script>

<div class="flex h-full w-full flex-col">
	<div class="relative flex-1 rounded-3xl bg-gray-950 p-5">
		<div class="absolute inset-x-0 top-0 z-10 mx-1 flex items-center justify-end gap-2 p-5">
			{#if connectState === 'disconnected'}
				<button onclick={connect}>
					<RefreshCcw class="icon-default" />
				</button>
				<div class="flex-1"></div>
			{/if}
			<button
				class="px-1 py-0.5 font-mono text-gray hover:bg-gray hover:text-white"
				onclick={() => {
					envDialog.show();
				}}>$ENV_VARS</button
			>
			<span
				class="font-mono uppercase"
				class:text-red-400={connectState === 'disconnected'}
				class:animate-pulse={connectState === 'connecting'}
				class:text-gray={connectState === 'connected'}>{connectState}</span
			>
			<button
				onclick={closeTerm}
				class="ms-4 font-mono text-gray hover:text-black hover:dark:text-white"
			>
				X
			</button>
		</div>
		<div class="m-2" bind:this={terminalContainer}></div>
	</div>
</div>

<Env bind:this={envDialog} />

<style lang="postcss">
	:global {
		.xterm > div {
			@apply scrollbar-none;
		}
	}
</style>
