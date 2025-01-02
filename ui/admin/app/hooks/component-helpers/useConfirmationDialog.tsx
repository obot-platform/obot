import { useCallback, useState } from "react";

import { noop } from "~/lib/utils";

import { ConfirmationDialogProps } from "~/components/composed/ConfirmationDialog";

type Handler = (e: React.MouseEvent<HTMLButtonElement>) => void;
type AsyncHandler = (
    e: React.MouseEvent<HTMLButtonElement>
) => Promise<unknown>;

export function useConfirmationDialog(
    baseProps?: Partial<ConfirmationDialogProps>
) {
    const [props, setProps] = useState<ConfirmationDialogProps>({
        title: "Are you sure?",
        onConfirm: noop,
        open: false,
        ...baseProps,
    });

    const updateProps = useCallback(
        (props: Partial<ConfirmationDialogProps>) =>
            setProps((prev) => ({ ...prev, ...props })),
        []
    );

    const intercept = useCallback(
        (handler: Handler, props?: Partial<ConfirmationDialogProps>) =>
            updateProps({
                onConfirm: handler,
                onCancel: () => updateProps({ open: false }),
                open: true,
                onOpenChange: (open) => updateProps({ open }),
                ...props,
            }),
        [updateProps]
    );

    const interceptAsync = useCallback(
        (handler: AsyncHandler, props?: Partial<ConfirmationDialogProps>) =>
            intercept(
                async (e) => {
                    await handler(e);
                    updateProps({ open: false });
                },
                { closeOnConfirm: false, ...props }
            ),
        [intercept, updateProps]
    );

    return { dialogProps: props, intercept, interceptAsync };
}
