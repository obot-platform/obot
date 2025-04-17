import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { PictureInPicture2Icon } from "lucide-react";
import { useMemo } from "react";
import useSWR from "swr";

import { Model, ModelUsage, getModelUsageLabel } from "~/lib/model/models";
import { ModelProvider } from "~/lib/model/providers";
import { ModelApiService } from "~/lib/service/api/modelApiService";

import { ProviderIcon } from "~/components/providers/ProviderIcon";
import { DataTable } from "~/components/composed/DataTable";
import { UpdateModelActive } from "~/components/model/UpdateModelActive";
import { UpdateModelUsage } from "~/components/model/UpdateModelUsage";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";
import { ScrollArea } from "~/components/ui/scroll-area";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

type ModelsConfigureProps = {
	modelProvider: ModelProvider;
};

const columnHelper = createColumnHelper<Model>();

export function ModelProvidersModels({ modelProvider }: ModelsConfigureProps) {
	const {
		data: modelsData,
		isLoading,
		mutate,
	} = useSWR(ModelApiService.getModels.key(), ModelApiService.getModels, {
		revalidateOnFocus: false,
	});

	const models = useMemo(() => {
		return (
			modelsData?.filter((model) => model.modelProvider === modelProvider.id) ??
			[]
		);
	}, [modelsData, modelProvider.id]);

	const handleModelActiveChange = (id: string, active: boolean) => {
		const updatedModelIndex = modelsData?.findIndex((model) => model.id === id);
		if (updatedModelIndex && updatedModelIndex !== -1 && modelsData) {
			modelsData[updatedModelIndex].active = active;
			mutate(modelsData, { revalidate: false });
		}
	};

	const handleModelUsageChange = (id: string, usage: ModelUsage) => {
		const updatedModelIndex = modelsData?.findIndex((model) => model.id === id);
		if (updatedModelIndex && updatedModelIndex !== -1 && modelsData) {
			modelsData[updatedModelIndex].usage = usage;
			mutate(modelsData, { revalidate: false });
		}
	};

	return (
		<Dialog>
			<Tooltip>
				<TooltipTrigger asChild>
					<DialogTrigger asChild>
						<Button size="icon" variant="ghost">
							<PictureInPicture2Icon />
						</Button>
					</DialogTrigger>
				</TooltipTrigger>
				<TooltipContent>{modelProvider.name} Models</TooltipContent>
			</Tooltip>

			<DialogDescription hidden>
				Configure & View Models of a Model Provider.
			</DialogDescription>

			<DialogContent
				className="gap-0 p-0"
				classNames={{
					content: "max-w-4xl",
				}}
			>
				<DialogHeader className="space-y-0 border-b border-b-secondary">
					<DialogTitle className="flex items-center gap-2 px-6 py-4">
						<ProviderIcon provider={modelProvider} /> {modelProvider.name}{" "}
						Models
					</DialogTitle>
				</DialogHeader>
				<ScrollArea className="h-[50vh]">
					{!isLoading && (
						<div className="px-6">
							<DataTable
								columns={getColumns()}
								data={models}
								sort={[{ id: "name", desc: false }]}
							/>
						</div>
					)}
				</ScrollArea>
			</DialogContent>
		</Dialog>
	);

	function getColumns(): ColumnDef<Model, string>[] {
		return [
			columnHelper.accessor((model) => model.name, {
				id: "name",
				header: "Model",
			}),
			columnHelper.accessor((model) => getModelUsageLabel(model.usage) || "", {
				id: "usage",
				header: "Usage",
				cell: ({ row }) => {
					return (
						<UpdateModelUsage
							model={row.original}
							key={row.original.id}
							onChange={(usage) =>
								handleModelUsageChange(row.original.id, usage)
							}
						/>
					);
				},
			}),
			columnHelper.display({
				id: "active",
				header: "Active",
				cell: ({ row }) => (
					<div className="flex justify-center">
						<UpdateModelActive
							model={row.original}
							key={row.original.id}
							onChange={(active) =>
								handleModelActiveChange(row.original.id, active)
							}
						/>
					</div>
				),
			}),
		];
	}
}
