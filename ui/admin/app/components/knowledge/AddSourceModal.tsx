import { FC, useState } from "react";

import { KNOWLEDGE_TOOL } from "~/lib/model/agents";
import { KnowledgeSourceType } from "~/lib/model/knowledge";

import KnowledgeSourceAvatar from "~/components/knowledge/KnowledgeSourceAvatar";
import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";

interface AddSourceModalProps {
	sourceType: KnowledgeSourceType;
	startPolling: () => void;
	isOpen: boolean;
	onOpenChange: (open: boolean) => void;
	addTool: (tool: string) => void;
	onAddWebsite: (website: string) => void;
	onAddOneDrive: (link: string) => void;
}

export const AddSourceModal: FC<AddSourceModalProps> = ({
	sourceType,
	startPolling,
	isOpen,
	onOpenChange,
	addTool,
	onAddWebsite,
	onAddOneDrive,
}) => {
	const [newWebsite, setNewWebsite] = useState("");
	const [newLink, setNewLink] = useState("");

	const handleAddWebsite = async () => {
		if (newWebsite) {
			onAddWebsite(newWebsite);
			setNewWebsite("");
		}
	};

	const handleAddOneDrive = async () => {
		if (newLink) {
			onAddOneDrive(newLink);
			setNewLink("");
		}
	};

	const handleAdd = async () => {
		if (sourceType === KnowledgeSourceType.Website) {
			await handleAddWebsite();
		} else if (sourceType === KnowledgeSourceType.OneDrive) {
			await handleAddOneDrive();
		}
		addTool(KNOWLEDGE_TOOL);
		startPolling();
		onOpenChange(false);
	};

	return (
		<Dialog open={isOpen} onOpenChange={onOpenChange}>
			<DialogContent aria-describedby="add-source-modal" className="max-w-2xl">
				<DialogTitle className="mb-4 flex flex-row items-center justify-between text-xl font-semibold">
					<div className="flex flex-row items-center">
						<KnowledgeSourceAvatar knowledgeSourceType={sourceType} />
						Add {sourceType}
					</div>
				</DialogTitle>
				<div className="mb-4">
					{sourceType !== KnowledgeSourceType.Notion && (
						<div className="mb-8 flex flex-col items-center justify-center">
							<div className="grid w-full grid-cols-2 items-center justify-center gap-2">
								<Label
									htmlFor="site"
									className="block text-center text-sm font-medium"
								>
									{sourceType === KnowledgeSourceType.Website && "Site"}
									{sourceType === KnowledgeSourceType.OneDrive && "Link URL"}
								</Label>
								<Input
									id="site"
									type="text"
									value={
										sourceType === KnowledgeSourceType.Website
											? newWebsite
											: newLink
									}
									onChange={(e) =>
										sourceType === KnowledgeSourceType.Website
											? setNewWebsite(e.target.value)
											: setNewLink(e.target.value)
									}
									placeholder={
										sourceType === KnowledgeSourceType.Website
											? "Enter website URL"
											: "Enter OneDrive folder link"
									}
									className="w-[250px] dark:bg-secondary"
								/>
							</div>
							{sourceType === KnowledgeSourceType.OneDrive && (
								<p className="mt-4 text-xs text-gray-500">
									For instructions on obtaining a OneDrive link, see{" "}
									<a
										href="https://support.microsoft.com/en-us/office/share-onedrive-files-and-folders-9fcc2f7d-de0c-4cec-93b0-a82024800c07#ID0EDBJ=Share_with_%22Copy_link%22"
										target="_blank"
										rel="noopener noreferrer"
										className="underline"
									>
										this document
									</a>
									.
								</p>
							)}
						</div>
					)}
					<div className="flex justify-end gap-2">
						<Button onClick={handleAdd} className="w-1/2" variant="secondary">
							OK
						</Button>
						<Button
							onClick={() => onOpenChange(false)}
							className="w-1/2"
							variant="secondary"
						>
							Cancel
						</Button>
					</div>
				</div>
			</DialogContent>
		</Dialog>
	);
};
