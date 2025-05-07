import type { Project, ProjectShare, ThreadManifest } from './services';

export function isImage(filename: string): boolean {
	return /\.(jpe?g|png|gif|bmp|webp)$/i.test(filename);
}

export function getProjectImage(
	project: Project | ProjectShare | ThreadManifest,
	isDarkMode: boolean
) {
	const imageUrl = isDarkMode
		? project.icons?.iconDark || project.icons?.icon
		: project.icons?.icon;

	return imageUrl ?? '/agent/images/obot_placeholder.webp'; // need placeholder image
}
