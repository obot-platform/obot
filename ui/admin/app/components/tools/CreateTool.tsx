import { PlusCircle } from "lucide-react";
import { useForm } from "react-hook-form";

import { CreateToolReference } from "~/lib/model/toolReferences";
import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";

import { Button } from "~/components/ui/button";
import { Input } from "~/components/ui/input";
import { useAsync } from "~/hooks/useAsync";

interface CreateToolProps {
    onSuccess: () => void;
}

export function CreateTool({ onSuccess }: CreateToolProps) {
    const { register, handleSubmit, reset } = useForm<CreateToolReference>();

    const { execute: onSubmit, isLoading } = useAsync(
        async (data: CreateToolReference) => {
            await ToolReferenceService.createToolReference({
                toolReference: { ...data, toolType: "tool" },
            });
            reset();
            onSuccess();
        }
    );

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
                <Button type="submit" disabled={isLoading}>
                    <PlusCircle className="w-4 h-4 mr-2" />
                    {isLoading ? "Creating..." : "Register Tool"}
                </Button>
            </div>
        </form>
    );
}
