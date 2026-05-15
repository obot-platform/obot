export function isImage(filename: string): boolean {
	return /\.(jpe?g|png|gif|bmp|webp)$/i.test(filename);
}
