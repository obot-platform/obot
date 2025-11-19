import { untrack } from 'svelte';

function defaultParser<T = string>(raw: string | null) {
	return raw as T;
}

type LocalStateParams<T = string> = {
	parse?: (raw: string | null) => T;
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
		const storedValue = get();

		untrack(() => {
			if (storedValue) {
				value = parse(storedValue);
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

	function get() {
		const local = localStorage.getItem(key);

		if (local) {
			return JSON.parse(local) as T;
		}

		return local as null;
	}

	function set(value: T | undefined | null) {
		if (value === undefined || value === null) {
			localStorage.removeItem(key);
			return value;
		}

		localStorage.setItem(key, JSON.stringify(value));

		return value;
	}
}
