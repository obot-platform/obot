import { Globe } from "lucide-react";

import { KnowledgeSourceType } from "~/lib/model/knowledge";
import { assetUrl, cn } from "~/lib/utils";

import { Avatar } from "~/components/ui/avatar";

export default function KnowledgeSourceAvatar({
	knowledgeSourceType,
	className,
}: {
	knowledgeSourceType: KnowledgeSourceType;
	className?: string;
}): React.ReactNode {
	const isOneDrive = knowledgeSourceType === KnowledgeSourceType.OneDrive;
	const isNotion = knowledgeSourceType === KnowledgeSourceType.Notion;
	const isWebsite = knowledgeSourceType === KnowledgeSourceType.Website;

	return (
		<>
			{isOneDrive && (
				<Avatar className={cn("mr-2 h-6 w-6", className)}>
					<img src={assetUrl("/onedrive.svg")} alt="OneDrive logo" />
				</Avatar>
			)}
			{isNotion && (
				<Avatar className={cn("mr-2 h-6 w-6", className)}>
					<img src={assetUrl("/notion.svg")} alt="Notion logo" />
				</Avatar>
			)}
			{isWebsite && (
				<Avatar className={cn("mr-2 h-6 w-6", className)}>
					<Globe className={cn("h-6 w-6", className)} />
				</Avatar>
			)}
		</>
	);
}
