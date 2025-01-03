interface TerminalState {
	open: boolean;
}

const state = $state<TerminalState>({
	open: false
});

export default state;
