import { useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import useSWR from "swr";

import { UpdateFileScannerConfig } from "~/lib/model/fileScannerConfig";
import { FileScannerConfigApiService } from "~/lib/service/api/fileScannerConfigApiService";
import { FileScannerProviderApiService } from "~/lib/service/api/fileScannerProviderApiService";

import { ComboBox } from "~/components/composed/ComboBox";
import {
	Form,
	FormControl,
	FormField,
	FormItem,
	FormLabel,
} from "~/components/ui/form";
import { useAsync } from "~/hooks/useAsync";

export function FileScannerConfigForm() {
	const { data: fileScannerConfig } = useSWR(
		FileScannerConfigApiService.getConfig.key,
		FileScannerConfigApiService.getConfig
	);

	const { data: fileScannerProviders } = useSWR(
		FileScannerProviderApiService.getFileScannerProviders.key,
		FileScannerProviderApiService.getFileScannerProviders
	);

	// Local state to track the currently selected option for immediate UI updates
	const [selectedOption, setSelectedOption] = useState<{
		id: string;
		providerName: string;
		name: string;
	} | null>(null);

	// Create a form instance
	const form = useForm({
		defaultValues: {
			provider: "",
		},
	});

	// Filter to only show configured providers
	const configuredProviders = useMemo(() => {
		if (!fileScannerProviders) return [];
		return fileScannerProviders.filter((provider) => provider.configured);
	}, [fileScannerProviders]);

	// Get the currently selected provider
	const selectedProvider = useMemo(() => {
		if (
			!fileScannerConfig ||
			!fileScannerConfig.providerName ||
			!fileScannerProviders
		)
			return null;
		return (
			fileScannerProviders.find(
				(provider) => provider.id === fileScannerConfig.providerName
			) || null
		);
	}, [fileScannerConfig, fileScannerProviders]);

	// Create options for the dropdown, including a "None" option
	const providerOptions = useMemo(() => {
		const options = configuredProviders.map((provider) => ({
			id: provider.id,
			providerName: provider.id,
			name: provider.name,
		}));

		// Add "None" option at the beginning
		options.unshift({
			id: "none",
			providerName: "",
			name: "None",
		});

		return options;
	}, [configuredProviders]);

	// Update the file scanner config when selection changes
	const updateConfig = useAsync(
		async (providerName: string) => {
			if (!fileScannerConfig) return;

			const update: UpdateFileScannerConfig = {
				...fileScannerConfig,
				providerName,
			};

			await FileScannerConfigApiService.updateConfig(update);
		},
		{
			onSuccess: () => {
				toast.success("File scanner provider updated");
			},
		}
	);

	// Update form value when provider changes
	const handleProviderChange = (
		option: { id: string; providerName: string; name: string } | null
	) => {
		const providerName = option?.providerName || "";
		form.setValue("provider", providerName);
		// Update local state for immediate UI update
		setSelectedOption(
			option
				? {
						id: option.id,
						providerName: option.providerName,
						name: option.name || "None",
					}
				: null
		);
		updateConfig.execute(providerName);
	};

	// Set initial form value based on selected provider
	useEffect(() => {
		if (selectedProvider) {
			form.setValue("provider", selectedProvider.id);
			// Initialize selectedOption state with the current provider
			setSelectedOption({
				id: selectedProvider.id,
				providerName: selectedProvider.id,
				name: selectedProvider.name,
			});
		} else if (fileScannerConfig) {
			form.setValue("provider", fileScannerConfig.providerName);
			// If no provider is selected but we have a providerName, set selectedOption to None
			if (fileScannerConfig.providerName === "") {
				setSelectedOption({ id: "none", providerName: "", name: "None" });
			}
		}
	}, [selectedProvider, fileScannerConfig, form]);

	return (
		<Form {...form}>
			<div className="space-y-6">
				<FormField
					control={form.control}
					name="provider"
					render={() => (
						<FormItem className="flex items-center justify-between gap-2 space-y-0">
							<FormLabel>Current Provider</FormLabel>
							<div className="flex flex-col">
								<FormControl>
									<ComboBox
										placeholder={selectedProvider?.name || "None"}
										onChange={handleProviderChange}
										options={providerOptions}
										value={selectedOption}
									/>
								</FormControl>
							</div>
						</FormItem>
					)}
				/>
			</div>
		</Form>
	);
}
