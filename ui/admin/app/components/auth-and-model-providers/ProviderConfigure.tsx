import { useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import useSWR, { mutate } from "swr";

import { AuthProvider, ModelProvider } from "~/lib/model/providers";
import {
	ForbiddenError,
	NotFoundError,
	UnauthorizedError,
} from "~/lib/service/api/apiErrors";
import { AuthProviderApiService } from "~/lib/service/api/authProviderApiService";
import { ModelApiService } from "~/lib/service/api/modelApiService";
import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";

import { ProviderForm } from "~/components/auth-and-model-providers/ProviderForm";
import { ProviderIcon } from "~/components/auth-and-model-providers/ProviderIcon";
import { CopyText } from "~/components/composed/CopyText";
import { DefaultModelAliasForm } from "~/components/model/DefaultModelAliasForm";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";

type ProviderConfigureProps = {
	provider: ModelProvider | AuthProvider;
	disabled: boolean;
};

export function ProviderConfigure({
	provider,
	disabled,
}: ProviderConfigureProps) {
	const [dialogIsOpen, setDialogIsOpen] = useState(false);
	const [showDefaultModelAliasForm, setShowDefaultModelAliasForm] =
		useState(false);

	const [loadingProviderId, setLoadingProviderId] = useState("");

	const getLoadingModelProviderModels = useSWR(
		provider.type === "modelprovider"
			? ModelProviderApiService.getModelProviderById.key(loadingProviderId)
			: null,
		({ providerId }) =>
			ModelProviderApiService.getModelProviderById(providerId),
		{
			revalidateOnFocus: false,
			refreshInterval: 2000,
		}
	);

	useEffect(() => {
		if (!loadingProviderId) return;

		const { isLoading, data } = getLoadingModelProviderModels;
		if (isLoading) return;

		if (data?.modelsBackPopulated) {
			setShowDefaultModelAliasForm(true);
			setLoadingProviderId("");
			// revalidate models to get back populated models
			mutate(ModelApiService.getModels.key());
		}
	}, [getLoadingModelProviderModels, loadingProviderId]);

	const handleDone = () => {
		setDialogIsOpen(false);
		setShowDefaultModelAliasForm(false);
	};

	return (
		<Dialog open={dialogIsOpen} onOpenChange={setDialogIsOpen}>
			<DialogTrigger asChild>
				<Button
					disabled={disabled}
					variant={provider.configured ? "secondary" : "accent"}
					className="mt-0 w-full"
				>
					{provider.configured ? "Modify" : "Configure"}
				</Button>
			</DialogTrigger>

			<DialogDescription hidden>Configure Provider</DialogDescription>

			<DialogContent
				className="max-w-2xl gap-0 p-0"
				hideCloseButton={loadingProviderId !== ""}
			>
				{loadingProviderId ? (
					<div className="flex items-center justify-center gap-1 p-2">
						<LoadingSpinner /> Loading {provider.name} Models...
					</div>
				) : showDefaultModelAliasForm ? (
					<div className="p-6">
						<DialogHeader>
							<DialogTitle className="flex items-center gap-2 pb-4">
								Configure Default Model Aliases
							</DialogTitle>
						</DialogHeader>
						<DialogDescription>
							When no model is specified, a default model is used for creating a
							new agent, workflow, or working with some tools, etc. Select your
							default models for the usage types below.
						</DialogDescription>
						<div className="mt-4">
							<DefaultModelAliasForm onSuccess={handleDone} />
						</div>
					</div>
				) : (
					<ProviderConfigureContent
						provider={provider}
						onSuccess={() =>
							provider.type === "modelprovider"
								? setLoadingProviderId(provider.id)
								: setDialogIsOpen(false)
						}
					/>
				)}
			</DialogContent>
		</Dialog>
	);
}

export function ProviderConfigureContent({
	provider,
	onSuccess,
}: {
	provider: ModelProvider | AuthProvider;
	onSuccess: () => void;
}) {
	const revealByIdFunc =
		provider.type === "modelprovider"
			? ModelProviderApiService.revealModelProviderById
			: AuthProviderApiService.revealAuthProviderById;

	const revealProvider = useSWR(
		revealByIdFunc.key(provider.id),
		async ({ providerId }) => {
			try {
				return await revealByIdFunc(providerId);
			} catch (error) {
				// no credential found or unauthorized = just return empty object
				if (
					error instanceof NotFoundError ||
					error instanceof UnauthorizedError ||
					error instanceof ForbiddenError
				) {
					return {};
				}
				// other errors = continue throw
				throw error;
			}
		}
	);

	const requiredParameters = provider.requiredConfigurationParameters;
	const optionalParameters = provider.optionalConfigurationParameters;
	const parameters = revealProvider.data;

	return (
		<>
			<DialogHeader className="space-y-0">
				<DialogTitle className="flex items-center gap-2 px-4 py-4">
					<ProviderIcon provider={provider} />{" "}
					{provider.configured
						? `Configure ${provider.name}`
						: `Set Up ${provider.name}`}
				</DialogTitle>
			</DialogHeader>

			{provider.description && (
				<DialogDescription className="px-4">
					<ReactMarkdown>{provider.description ?? ""}</ReactMarkdown>
				</DialogDescription>
			)}
			{provider.type === "authprovider" && (
				<DialogDescription className="flex items-center justify-center px-4">
					Note: the callback URL for this auth provider is
					<CopyText
						text={window.location.protocol + "//" + window.location.host + "/"}
						className="w-fit-content ml-1 max-w-full"
					/>
				</DialogDescription>
			)}
			{revealProvider.isLoading ? (
				<LoadingSpinner />
			) : (
				<ProviderForm
					provider={provider}
					onSuccess={onSuccess}
					parameters={parameters ?? {}}
					requiredParameters={requiredParameters ?? []}
					optionalParameters={optionalParameters ?? []}
				/>
			)}
		</>
	);
}
