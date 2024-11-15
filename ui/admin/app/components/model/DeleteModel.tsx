import { TrashIcon } from "lucide-react";
import { mutate } from "swr";

import { ModelApiService } from "~/lib/service/api/modelApiService";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { Button } from "~/components/ui/button";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";
import { useAsync } from "~/hooks/useAsync";

type DeleteModelProps = {
    id: string;
};

export function DeleteModel(props: DeleteModelProps) {
    const deleteModel = useAsync(ModelApiService.deleteModel, {
        onSuccess: () => mutate(ModelApiService.getModels.key()),
    });

    return (
        <TooltipProvider>
            <Tooltip>
                <ConfirmationDialog
                    title="Are you sure you want to delete this model?"
                    description="Doing so will break any tools or agents currently using it."
                    onConfirm={(e) => {
                        e.stopPropagation();
                        deleteModel.execute(props.id);
                    }}
                    confirmProps={{
                        variant: "destructive",
                        children: "Delete",
                    }}
                >
                    <TooltipTrigger asChild>
                        <Button
                            size="icon"
                            variant="ghost"
                            onClick={(e) => e.stopPropagation()}
                            disabled={deleteModel.isLoading}
                            loading={deleteModel.isLoading}
                        >
                            <TrashIcon />
                        </Button>
                    </TooltipTrigger>
                </ConfirmationDialog>

                <TooltipContent>Delete Model</TooltipContent>
            </Tooltip>
        </TooltipProvider>
    );
}
