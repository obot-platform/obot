export function downloadFile(data: Blob, fileName: string) {
	const url = URL.createObjectURL(data);
	const a = document.createElement("a");
	a.href = url;
	a.download = fileName;
	a.click();
}

export function downloadUrl(url: string, fileName: string) {
	const a = document.createElement("a");
	a.href = url;
	a.download = fileName;
	a.click();
}
