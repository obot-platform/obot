import { PlusCircle } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import useSWR from "swr";

import { CreateToolReference, ToolReference } from "~/lib/model/toolReferences";
import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";

import { Button } from "~/components/ui/button";
import { Input } from "~/components/ui/input";
import { useAsync } from "~/hooks/useAsync";

interface CreateToolProps {
    onError: (error: string) => void;
    onSuccess: () => void;
}

export function CreateTool({ onError, onSuccess }: CreateToolProps) {
    const { register, handleSubmit, reset } = useForm<CreateToolReference>();

    const [loadingToolId, setLoadingToolId] = useState("");
    const getLoadingTool = useSWR(
        loadingToolId
            ? ToolReferenceService.getToolReferenceById.key(loadingToolId)
            : null,
        ({ toolReferenceId }) =>
            ToolReferenceService.getToolReferenceById(toolReferenceId),
        {
            revalidateOnFocus: false,
            refreshInterval: 2000,
        }
    );

    const handleCreatedTool = useCallback(
        (loadedTool: ToolReference) => {
            setLoadingToolId("");
            reset();
            if (loadedTool.error) {
                onError(loadedTool.error);
            } else {
                toast.success(
                    `"${loadedTool.reference}" registered successfully.`
                );
                onSuccess();
            }
        },
        [onError, reset, onSuccess]
    );

    useEffect(() => {
        if (!loadingToolId) return;

        const { isLoading, data } = getLoadingTool;
        if (isLoading) return;

        if (data?.resolved) {
            handleCreatedTool(data);
        }
    }, [getLoadingTool, handleCreatedTool, loadingToolId]);

    const { execute: onSubmit, isLoading } = useAsync(
        async (data: CreateToolReference) => {
            const response = await ToolReferenceService.createToolReference({
                toolReference: { ...data, toolType: "tool" },
            });

            setLoadingToolId(response.id);
        }
    );

    const pending = isLoading || !!loadingToolId;
    return (
        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            <div>
                <Input
                    autoComplete="off"
                    {...register("reference", {
                        required: "Reference is required",
                    })}
                    placeholder="github.com/user/repo or https://example.com/tool.gpt"
                />
            </div>
            <div className="flex justify-end">
                <Button
                    type="submit"
                    disabled={pending}
                    loading={pending}
                    startContent={<PlusCircle />}
                >
                    Register Tool
                </Button>
            </div>
        </form>
    );
}
