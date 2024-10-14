import { useCallback, useState } from "react";

type UseDisclosureProps = {
    defaultIsOpen?: boolean;
    onClose?: () => void;
    onOpen?: () => void;
};

export function useDisclosure({
    defaultIsOpen = false,
    onClose,
    onOpen,
}: UseDisclosureProps = {}) {
    const [isOpen, setIsOpen] = useState(defaultIsOpen);

    const handleToggle = useCallback(() => {
        setIsOpen((prev) => !prev);

        if (isOpen) onClose?.();
        else onOpen?.();
    }, [isOpen, onClose, onOpen]);

    const handleSetOpen = useCallback(
        (open: boolean) => {
            setIsOpen(open);
            if (open) onOpen?.();
            else onClose?.();
        },
        [onClose, onOpen]
    );

    const onOpenChange = useCallback(
        (open?: boolean) => {
            if (open == null) handleToggle();
            else handleSetOpen(open);
        },
        [handleSetOpen, handleToggle]
    );

    const handleClose = useCallback(() => onOpenChange(false), [onOpenChange]);
    const handleOpen = useCallback(() => onOpenChange(true), [onOpenChange]);

    return { isOpen, onOpenChange, onClose: handleClose, onOpen: handleOpen };
}
