export function formatNumber(num: number): string {
	if (num >= 1000) {
		const thousands = num / 1000;
		return thousands % 1 === 0 ? `${thousands}k` : `${thousands.toFixed(1)}k`;
	}
	return num.toString();
}
