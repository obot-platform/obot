<script lang="ts">
	import {
		Channel,
		controlFrame,
		frame,
		parseFrame,
		terminalURL,
		type ControlMessage
	} from '$lib/services/hostedAgentTerminal';
	import type { Terminal } from '@xterm/xterm';
	import '@xterm/xterm/css/xterm.css';
	import { onMount } from 'svelte';

	interface Props {
		instanceID: string;
	}

	let { instanceID }: Props = $props();

	let container = $state<HTMLDivElement>();
	// Deliberately not $state: putting the xterm instance in a rune wraps it in a
	// reactive proxy, and xterm's internals do not survive being proxied. Nothing
	// in the markup depends on it, so a plain binding is all it needs.
	let terminal: Terminal | undefined;
	let status = $state<'connecting' | 'open' | 'closed'>('connecting');
	let error = $state('');

	const decoder = new TextDecoder();
	const encoder = new TextEncoder();

	// Always dark, regardless of Obot's theme. A terminal is not a document: the
	// programs run in it pick their own colours, and a TUI such as Codex draws
	// on the assumption of a dark background -- on a light one its dimmed text
	// and borders wash out to near-invisible.
	const theme = { background: '#0b0f19', foreground: '#e5e7eb', cursor: '#e5e7eb' };

	// xterm reaches for `self` as soon as it is imported, which fails when the
	// page is rendered on the server -- and dev builds do render on the server.
	// Importing it here keeps it out of the server bundle entirely; the type-only
	// import above is erased at build time.
	onMount(() => {
		let dispose: (() => void) | undefined;
		let cancelled = false;

		void (async () => {
			const [{ Terminal }, { FitAddon }] = await Promise.all([
				import('@xterm/xterm'),
				import('@xterm/addon-fit')
			]);
			// The component can be destroyed while the import is in flight, which
			// would otherwise attach a console to a container that is already gone.
			if (cancelled) return;
			dispose = start(Terminal, FitAddon);
		})();

		return () => {
			cancelled = true;
			dispose?.();
		};
	});

	function start(
		TerminalCtor: typeof import('@xterm/xterm').Terminal,
		FitAddonCtor: typeof import('@xterm/addon-fit').FitAddon
	) {
		terminal = new TerminalCtor({
			convertEol: true,
			cursorBlink: true,
			fontFamily:
				'ui-monospace, SFMono-Regular, Menlo, Monaco, "Cascadia Mono", "Liberation Mono", monospace',
			fontSize: 13,
			theme
		});
		const session = terminal;
		const fit = new FitAddonCtor();
		session.loadAddon(fit);
		session.open(container!);
		fit.fit();

		// The server needs the size before it attaches, so the first thing the
		// program draws already fits and does not have to be redrawn.
		const socket = new WebSocket(terminalURL(instanceID, session.cols, session.rows));
		socket.binaryType = 'arraybuffer';

		function send(message: Uint8Array) {
			if (socket.readyState === WebSocket.OPEN) {
				// Copying through slice() keeps the ArrayBuffer exact; a subarray of
				// a larger buffer would be sent in full.
				socket.send(message.slice().buffer);
			}
		}

		socket.addEventListener('open', () => {
			status = 'open';
			session.focus();
		});

		socket.addEventListener('message', (event) => {
			const parsed = parseFrame(new Uint8Array(event.data as ArrayBuffer));
			if (!parsed) return;

			switch (parsed.channel) {
				case Channel.stdout:
				case Channel.stderr:
					session.write(parsed.payload);
					break;
				case Channel.control: {
					let control: ControlMessage;
					try {
						control = JSON.parse(decoder.decode(parsed.payload));
					} catch {
						return;
					}
					// An error here is Obot's, not the sandbox's: a failed attach, a
					// sandbox that stopped. Showing it beside the console rather than
					// writing it into the scrollback keeps the two distinguishable.
					if (control.type === 'error' && control.message) {
						error = control.message;
					}
					break;
				}
			}
		});

		socket.addEventListener('close', () => {
			status = 'closed';
		});

		socket.addEventListener('error', () => {
			// A websocket error carries no detail by design; the close event that
			// follows is what actually ends the session.
			if (!error) error = 'The terminal connection failed.';
		});

		const input = session.onData((data) => send(frame(Channel.stdin, encoder.encode(data))));

		// Binary input arrives as a string of char codes rather than text, so it
		// is passed through without an encoder that would mangle it.
		const binary = session.onBinary((data) => {
			const bytes = new Uint8Array(data.length);
			for (let i = 0; i < data.length; i++) bytes[i] = data.charCodeAt(i) & 0xff;
			send(frame(Channel.stdin, bytes));
		});

		const resized = session.onResize(({ cols, rows }) =>
			send(controlFrame({ type: 'resize', cols, rows }))
		);

		// fit() reads the container, so it has to run after the browser has laid
		// it out; xterm then reports the new size and the resize is sent above.
		const observer = new ResizeObserver(() => {
			try {
				fit.fit();
			} catch {
				// A container with no size yet — during a route transition, say —
				// cannot be fitted. The next observation will do it.
			}
		});
		observer.observe(container!);

		return () => {
			observer.disconnect();
			input.dispose();
			binary.dispose();
			resized.dispose();
			socket.close();
			session.dispose();
			terminal = undefined;
		};
	}
</script>

<div class="flex min-h-0 grow flex-col">
	{#if error}
		<div class="notification-error mb-2 text-sm" role="alert">{error}</div>
	{/if}
	<div
		class="relative min-h-0 grow overflow-hidden rounded-lg bg-[#0b0f19] p-2"
		class:opacity-60={status !== 'open'}
	>
		<div class="size-full" bind:this={container}></div>
		{#if status === 'connecting'}
			<p
				class="text-muted-content pointer-events-none absolute inset-x-0 top-1/2 text-center text-sm"
			>
				Connecting…
			</p>
		{/if}
	</div>
	<p class="text-muted-content mt-2 text-xs">
		{#if status === 'open'}
			Attached to the sandbox console. Anything you type goes to the running agent.
		{:else if status === 'closed'}
			The session ended. Go back and open the terminal again to reattach.
		{:else}
			Attaching to the sandbox console…
		{/if}
	</p>
</div>
