import type { Project, ProjectShare } from './services';

export function isImage(filename: string): boolean {
	return /\.(jpe?g|png|gif|bmp|webp)$/i.test(filename);
}

export function getProjectImage(project: Project | ProjectShare, isDarkMode: boolean) {
	const imageUrl = isDarkMode
		? project.icons?.iconDark || project.icons?.icon
		: project.icons?.icon;

	return imageUrl ?? '/agent/images/placeholder.webp'; // need placeholder image
}
