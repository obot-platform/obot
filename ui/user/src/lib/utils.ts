// Simple delay function
export function delay(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

// Simple throttle function
export function throttle<T extends (...args: Parameters<T>) => ReturnType<T>>(
	func: T,
	delay: number
): T {
	let timeoutId: number | null = null;
	return ((...args: Parameters<T>) => {
		if (timeoutId) return;
		timeoutId = setTimeout(() => {
			func(...args);
			timeoutId = null;
		}, delay);
	}) as T;
}

// Poll a function until it returns true, or until a timeout is reached.
// Returns when the function returns true.
// Throws an exception if the timeout is reached before the function returns true.
export async function poll(
	pollFn: () => Promise<boolean>,
	options: {
		interval?: number;
		maxTimeout?: number;
	} = {}
): Promise<void> {
	const { interval = 1000, maxTimeout = 30000 } = options;
	const startTime = Date.now();

	while (true) {
		if (await pollFn()) {
			return;
		}

		if (Date.now() - startTime >= maxTimeout) {
			throw new Error(`Poll timeout after ${maxTimeout}ms`);
		}

		await delay(interval);
	}
}
