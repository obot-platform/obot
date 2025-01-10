import { Avatar } from "@radix-ui/react-avatar";
import { GlobeIcon, PlusIcon, UploadIcon } from "lucide-react";

import { KnowledgeSourceType } from "~/lib/model/knowledge";
import { assetUrl } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";

interface AddKnowledgeButtonProps {
	disabled?: boolean;
	onUploadFiles: () => void;
	onAddSource: (sourceType: KnowledgeSourceType) => void;
	hasExistingNotion: boolean;
}

export function AddKnowledgeButton({
	disabled,
	onUploadFiles,
	onAddSource,
	hasExistingNotion,
}: AddKnowledgeButtonProps) {
	return (
		<div className="flex w-full justify-end">
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						variant="ghost"
						className="flex items-center gap-2"
						disabled={disabled}
					>
						<PlusIcon className="h-5 w-5 text-foreground" />
						Add Knowledge
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent side="top">
					<DropdownMenuItem onClick={onUploadFiles} className="cursor-pointer">
						<div className="flex items-center">
							<UploadIcon className="mr-2 h-4 w-4" />
							Local Files
						</div>
					</DropdownMenuItem>
					<DropdownMenuItem
						onClick={() => onAddSource(KnowledgeSourceType.OneDrive)}
						className="cursor-pointer"
					>
						<div className="flex flex-row justify-center">
							<div className="flex flex-row justify-center">
								<div className="flex items-center justify-center">
									<Avatar className="mr-2 h-4 w-4">
										<img src={assetUrl("/onedrive.svg")} alt="OneDrive logo" />
									</Avatar>
								</div>
								<span>OneDrive</span>
							</div>
						</div>
					</DropdownMenuItem>
					<DropdownMenuItem
						onClick={() => onAddSource(KnowledgeSourceType.Notion)}
						className="cursor-pointer"
						disabled={hasExistingNotion}
					>
						<div className="flex flex-row justify-center">
							<Avatar className="mr-2 h-4 w-4">
								<img src={assetUrl("/notion.svg")} alt="Notion logo" />
							</Avatar>
							Notion
						</div>
					</DropdownMenuItem>
					<DropdownMenuItem
						onClick={() => onAddSource(KnowledgeSourceType.Website)}
						className="cursor-pointer"
					>
						<div className="flex justify-center">
							<GlobeIcon className="mr-2 h-4 w-4" />
							Website
						</div>
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
		</div>
	);
}
