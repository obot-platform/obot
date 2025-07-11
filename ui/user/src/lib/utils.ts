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
