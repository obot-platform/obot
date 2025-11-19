import { untrack } from 'svelte';

function defaultParser<T = string>(value: string | null): T | null {
	return value ? JSON.parse(value) : null;
}

type LocalStateParams<T = string> = {
	parse?: (value: string | null) => T | null;
};

export function localState<T = string>(
	key: string,
	defaultValue: T,
	{ parse = defaultParser }: LocalStateParams<T> = {}
) {
	let value = $state<T | undefined | null>();
	let isReady = $state(false);

	let shouldCaptureUpdates = false;

	$effect(() => {
		const local = localStorage.getItem(key);

		untrack(() => {
			if (local) {
				value = parse(local);
			} else {
				value = set(defaultValue);
			}

			isReady = true;
			shouldCaptureUpdates = true;
		});
	});

	$effect(() => {
		if (!shouldCaptureUpdates) return;
		if (!localStorage) return;

		set(value);
	});

	return {
		get current() {
			return value;
		},
		set current(v) {
			value = v as T;
		},
		get isReady() {
			return isReady;
		}
	};

	function set(value: T | undefined | null) {
		if (value === undefined || value === null) {
			localStorage.removeItem(key);
			return value;
		}

		localStorage.setItem(key, JSON.stringify(value));

		return value;
	}
}
