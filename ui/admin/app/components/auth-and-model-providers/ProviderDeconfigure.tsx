import { useState } from "react";
import { toast } from "sonner";
import { mutate } from "swr";

import { AuthProvider, ModelProvider } from "~/lib/model/providers";
import { AuthProviderApiService } from "~/lib/service/api/authProviderApiService";
import { ModelApiService } from "~/lib/service/api/modelApiService";
import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";

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
import { DropdownMenuItem } from "~/components/ui/dropdown-menu";
import { useAsync } from "~/hooks/useAsync";

export function ProviderDeconfigure({
	provider,
}: {
	provider: ModelProvider | AuthProvider;
}) {
	const [open, setOpen] = useState(false);
	const handleDeconfigure = async () => {
		deconfigure.execute(provider.id);
	};

	const deconfigure = useAsync(
		provider.type === "modelprovider"
			? ModelProviderApiService.deconfigureModelProviderById
			: AuthProviderApiService.deconfigureAuthProviderById,
		{
			onSuccess: () => {
				toast.success(`${provider.name} deconfigured.`);
				mutate(
					provider.type === "modelprovider"
						? ModelProviderApiService.getModelProviders.key()
						: AuthProviderApiService.getAuthProviders.key()
				);
				mutate(
					provider.type === "modelprovider"
						? ModelApiService.getModels.key()
						: null
				);
			},
			onError: () => toast.error(`Failed to deconfigure ${provider.name}`),
		}
	);

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger>
				<DropdownMenuItem
					onClick={(event) => {
						event.preventDefault();
						setOpen(true);
					}}
					className="text-destructive"
				>
					Deconfigure Provider
				</DropdownMenuItem>
			</DialogTrigger>

			<DialogDescription hidden>Configure Provider</DialogDescription>

			<DialogContent hideCloseButton>
				<DialogHeader>
					<DialogTitle>Deconfigure {provider.name}</DialogTitle>
				</DialogHeader>
				<p>{warningMessage(provider.type)}</p>
				<p>
					Are you sure you want to deconfigure <b>{provider.name}</b>?
				</p>
				<DialogFooter>
					<div className="flex w-full items-center justify-center gap-10 pt-4">
						<DialogClose asChild>
							<Button className="w-1/2" variant="outline">
								Cancel
							</Button>
						</DialogClose>
						<DialogClose asChild>
							<Button
								className="w-1/2"
								onClick={handleDeconfigure}
								variant="destructive"
							>
								Confirm
							</Button>
						</DialogClose>
					</div>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

function warningMessage(t?: string): string | undefined {
	switch (t) {
		case "modelprovider":
			return "Deconfiguring this model provider will remove all models associated with it and reset it to its unconfigured state. You will need to set up the model provider once again to use it.";
		case "authprovider":
			return "Deconfiguring this auth provider will sign out all users who are using it and reset it to its unconfigured state. You will need to set up the auth provider once again to use it.";
	}
}
