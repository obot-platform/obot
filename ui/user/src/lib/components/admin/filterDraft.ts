/** Adopt saved values only where the user has not edited the submitted draft further. */
export function reconcileFilterDraft<T>(current: T, submitted: T, saved: T): T {
	if (JSON.stringify(current) === JSON.stringify(submitted)) return structuredClone(saved);
	if (
		!current ||
		!submitted ||
		!saved ||
		typeof current !== 'object' ||
		typeof submitted !== 'object' ||
		typeof saved !== 'object' ||
		Array.isArray(current) ||
		Array.isArray(submitted) ||
		Array.isArray(saved)
	)
		return current;
	const draft = current as Record<string, unknown>;
	const previous = submitted as Record<string, unknown>;
	const persisted = saved as Record<string, unknown>;
	return Object.fromEntries(
		[...new Set([...Object.keys(draft), ...Object.keys(persisted)])].map((key) => [
			key,
			reconcileFilterDraft(draft[key], previous[key], persisted[key])
		])
	) as T;
}
