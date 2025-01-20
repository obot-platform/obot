import { ComponentProps, ReactNode } from "react";

import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";

export type ConfirmationDialogProps = ComponentProps<typeof Dialog> & {
	children?: ReactNode;
	title: ReactNode;
	description?: ReactNode;
	content?: ReactNode;
	onConfirm: (e: React.MouseEvent<HTMLButtonElement>) => void;
	onCancel?: (e: React.MouseEvent<HTMLButtonElement>) => void;
	confirmProps?: Omit<Partial<ComponentProps<typeof Button>>, "onClick">;
	cancelProps?: Omit<Partial<ComponentProps<typeof Button>>, "onClick">;
	closeOnConfirm?: boolean;
};

export function ConfirmationDialog({
	children,
	title,
	description,
	content,
	onConfirm,
	onCancel,
	confirmProps,
	cancelProps,
	closeOnConfirm = true,
	...dialogProps
}: ConfirmationDialogProps) {
	return (
		<Dialog {...dialogProps}>
			{children && <DialogTrigger asChild>{children}</DialogTrigger>}

			<DialogContent onClick={(e) => e.stopPropagation()}>
				<DialogHeader>
					<DialogTitle>{title}</DialogTitle>
				</DialogHeader>

				<DialogDescription>{description}</DialogDescription>

				{content}

				<DialogFooter>
					<DialogClose onClick={onCancel} asChild>
						<Button variant="secondary" {...cancelProps}>
							{cancelProps?.children ?? "Cancel"}
						</Button>
					</DialogClose>

					{closeOnConfirm ? (
						<DialogClose onClick={onConfirm} asChild>
							<Button {...confirmProps}>
								{confirmProps?.children ?? "Confirm"}
							</Button>
						</DialogClose>
					) : (
						<Button {...confirmProps} onClick={onConfirm}>
							{confirmProps?.children ?? "Confirm"}
						</Button>
					)}
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
