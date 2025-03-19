<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import '@xterm/xterm/css/xterm.css';
	import { RefreshCcw } from 'lucide-svelte';
	import { term } from '$lib/stores';
	import Env from '$lib/components/terminal/Env.svelte';
	import type { Project } from '$lib/services';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
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
		let size = { cols: 0, rows: 0 };
		term.loadAddon(fitAddon);
		fitAddon.fit();

		const resize = () => {
			const newSize = fitAddon.proposeDimensions();
			if (
				newSize &&
				(size.cols !== newSize.cols || size.rows !== newSize.rows) &&
				connectState === 'connected'
			) {
				fitAddon.fit();
				socket.send(new Blob(['\x01' + JSON.stringify(newSize)], { type: 'application/json' }));
				size = newSize;
			}
		};

		term.open(terminalContainer);

		new ResizeObserver(() => {
			setTimeout(resize);
		}).observe(terminalContainer);

		const url =
			window.location.protocol.replaceAll('http', 'ws') +
			`//${window.location.host}/api/assistants/${project.assistantID}/projects/${project.id}/shell`;
		const socket = new WebSocket(url);
		connectState = 'connecting';
		socket.onmessage = (event) => {
			if (event.data instanceof Blob) {
				event.data.text().then((text) => {
					term.write(text);
				});
			}
		};
		socket.onopen = () => {
			connectState = 'connected';
			resize();
			term.focus();
			socket.send(new Blob(['\x00\x0C']));
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
			socket.send(new Blob(['\x00' + data]));
		});

		close = () => {
			socket.close();
			term.dispose();
		};
	}
</script>

<div class="flex h-full w-full flex-col">
	<div class="relative flex h-full w-full flex-col rounded-3xl bg-gray-950 p-5">
		{#if connectState === 'disconnected'}
			<div
				class="pointer-events-none absolute inset-0 z-20 flex h-full w-full items-center justify-center"
			>
				<button
					onclick={connect}
					class="pointer-events-auto rounded-lg border-2 border-red-400 bg-gray-950 p-3"
				>
					<RefreshCcw class="icon-default" />
				</button>
			</div>
		{/if}
		<div class="absolute inset-x-0 top-0 z-10 mx-1 flex items-center justify-end gap-2 p-5">
			<button
				class="text-gray hover:bg-gray px-1 py-0.5 font-mono hover:text-white"
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
			<button onclick={closeTerm} class="text-gray ms-4 font-mono hover:text-white"> X </button>
		</div>
		<div class="m-2 flex h-full w-full" bind:this={terminalContainer}></div>
	</div>
</div>

<Env bind:this={envDialog} {project} />

<style lang="postcss">
	:global {
		.xterm > div {
			scrollbar-width: none;
			&::-webkit-scrollbar {
				display: none;
			}
		}
	}
</style>
