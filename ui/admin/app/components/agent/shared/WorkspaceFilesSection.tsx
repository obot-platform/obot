import { FileIcon, PlusIcon, TrashIcon } from "lucide-react";
import { useRef } from "react";
import useSWR from "swr";

import { AgentService } from "~/lib/service/api/agentService";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { Truncate } from "~/components/composed/typography";
import { Button } from "~/components/ui/button";
import { CardDescription } from "~/components/ui/card";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";
import { useConfirmationDialog } from "~/hooks/component-helpers/useConfirmationDialog";
import { useAsync } from "~/hooks/useAsync";
import { useMultiAsync } from "~/hooks/useMultiAsync";

type WorkspaceFilesSectionProps = {
	entityId: string;
};

export function WorkspaceFilesSection({
	entityId,
}: WorkspaceFilesSectionProps) {
	const inputRef = useRef<HTMLInputElement>(null);
	const { dialogProps, interceptAsync } = useConfirmationDialog();

	const { data: files, mutate: refresh } = useSWR(
		AgentService.getWorkspaceFiles.key(entityId),
		({ agentId }) => AgentService.getWorkspaceFiles(agentId)
	);

	const deleteFile = useAsync(AgentService.deleteWorkspaceFile, {
		onSuccess: (fileName) =>
			// optomistic update
			refresh((files) => files?.filter((f) => f.name !== fileName)),
	});

	const uploadFiles = useMultiAsync(
		async (_index: number, file: File) =>
			await AgentService.uploadWorkspaceFile(entityId, file),
		{
			onSuccess: (data) =>
				// optomistic update
				refresh((files) => [
					// remove conflicting files
					...(files?.filter((f) => !data.includes(f.name)) ?? []),
					// add new files
					...data.map((f) => ({ name: f })),
				]),
		}
	);

	const startUpload = (files: FileList) => {
		if (!files.length) return;

		uploadFiles.execute(Array.from(files).map((file) => [file]));

		if (inputRef.current) inputRef.current.value = "";
	};

	const uploading = uploadFiles.states.some((s) => s.isLoading);

	return (
		<div className="m-4 space-y-4 p-4">
			<h4 className="border-b pb-2">Workspace Files</h4>

			<CardDescription>
				Add environment variables that will be available to all tools as key
				value pairs.
			</CardDescription>

			<div className="flex flex-col gap-2">
				{files?.map((file) => (
					<div
						key={file.name}
						className="flex items-center justify-between gap-2"
					>
						<div className="flex items-center gap-2">
							<FileIcon className="size-5" />
							<Truncate>{file.name}</Truncate>
						</div>

						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									size="icon"
									variant="ghost"
									onClick={() =>
										interceptAsync(() =>
											deleteFile.executeAsync(entityId, file.name)
										)
									}
									startContent={<TrashIcon className="size-5" />}
								/>
							</TooltipTrigger>

							<TooltipContent>Remove File</TooltipContent>
						</Tooltip>
					</div>
				))}

				<Button
					variant="ghost"
					className="self-end"
					startContent={<PlusIcon />}
					onClick={() => inputRef.current?.click()}
					loading={uploading}
					disabled={uploading}
				>
					Upload Files
				</Button>

				<input
					type="file"
					ref={inputRef}
					multiple
					className="hidden"
					onChange={(e) => {
						if (!e.target.files) return;
						startUpload(e.target.files);
					}}
				/>

				<ConfirmationDialog
					{...dialogProps}
					title="Remove File?"
					description="Are you sure you want to remove this file? this action cannot be undone."
					confirmProps={{
						loading: deleteFile.isLoading,
						disabled: deleteFile.isLoading,
						variant: "destructive",
					}}
				/>
			</div>
		</div>
	);
}
