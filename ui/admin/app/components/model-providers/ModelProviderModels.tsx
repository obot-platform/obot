import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { SettingsIcon } from "lucide-react";
import useSWR from "swr";

import { ModelProvider } from "~/lib/model/modelProviders";
import { Model, getModelUsageLabel } from "~/lib/model/models";
import { ModelApiService } from "~/lib/service/api/modelApiService";

import { DataTable } from "~/components/composed/DataTable";
import { ModelProviderIcon } from "~/components/model-providers/ModelProviderIcon";
import { DeleteModel } from "~/components/model/DeleteModel";
import { Badge } from "~/components/ui/badge";
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
    const getModels = useSWR(
        ModelApiService.getModels.key(),
        ModelApiService.getModels
    );

    const models =
        getModels.data?.filter(
            (model) => model.modelProvider === modelProvider.id
        ) ?? [];
    return (
        <Dialog>
            <Tooltip>
                <TooltipTrigger asChild>
                    <DialogTrigger asChild>
                        <Button size="icon" variant="ghost">
                            <SettingsIcon />
                        </Button>
                    </DialogTrigger>
                </TooltipTrigger>
                <TooltipContent>{modelProvider.name} Models</TooltipContent>
            </Tooltip>

            <DialogDescription hidden>
                Configure & View Models of a Modal Provider.
            </DialogDescription>

            <DialogContent
                className="p-0 gap-0"
                classNames={{
                    content: "max-w-4xl",
                }}
            >
                <DialogHeader className="space-y-0 border-b-secondary border-b">
                    <DialogTitle className="flex items-center gap-2 px-6 py-4">
                        <ModelProviderIcon modelProvider={modelProvider} />{" "}
                        {modelProvider.name} Models
                    </DialogTitle>
                </DialogHeader>
                <ScrollArea className="h-[50vh]">
                    <DataTable
                        columns={getColumns()}
                        data={models}
                        sort={[{ id: "name", desc: false }]}
                    />
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
            columnHelper.display({
                id: "usage",
                header: "Usage",
                cell: ({ row }) =>
                    getModelUsageLabel(row.original.usage) ? (
                        <Badge variant="outline">
                            {getModelUsageLabel(row.original.usage)}
                        </Badge>
                    ) : null,
            }),
            columnHelper.display({
                id: "actions",
                cell: ({ row }) => (
                    <div className="flex justify-end">
                        <DeleteModel id={row.original.id} />
                    </div>
                ),
            }),
        ];
    }
}
