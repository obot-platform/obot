import {
    ReactNode,
    createContext,
    useCallback,
    useContext,
    useState,
} from "react";
import useSWR, { mutate } from "swr";

import { ModelProvider } from "~/lib/model/modelProviders";
import { ModelProviderService } from "~/lib/service/api/modelProviderApiService";

import { useAsync } from "~/hooks/useAsync";

interface ModelProviderContextType {
    modelProviders: ModelProvider[];
    configured: boolean;
    configureModelProvider: (
        modelProviderId: string,
        value: Record<string, string>
    ) => void;
    isUpdating: boolean;
    error?: unknown;
    lastUpdated?: Date;
}

const ModelProviderContext = createContext<
    ModelProviderContextType | undefined
>(undefined);

type ModelProviderName = keyof typeof readableNameMapToModelProvider;

export const readableNameMapToModelProvider = {
    "ollama-model-provider": "Ollama",
    "anthropic-model-provider": "Anthropic",
    "voyage-model-provider": "Voyage",
    "openai-model-provider": "OpenAI",
    "azure-openai-model-provider": "Azure OpenAI",
};

export function ModelProviderProvider({ children }: { children: ReactNode }) {
    const getModelProviders = useSWR(
        ModelProviderService.getModelProviders.key(),
        () => ModelProviderService.getModelProviders(),
        { fallbackData: [] }
    );

    const [lastUpdated, setLastSaved] = useState<Date>();

    const handleConfigureModelProvider = useCallback(
        (modelProviderId: string, values: Record<string, string>) =>
            ModelProviderService.configureModelProviderById(
                modelProviderId,
                values
            )
                .then(() => {
                    getModelProviders.mutate();
                    mutate(ModelProviderService.getModelProviders.key());
                    setLastSaved(new Date());
                })
                .catch(console.error),
        [getModelProviders]
    );

    const configureModelProvider = useAsync(handleConfigureModelProvider);
    const configured = getModelProviders.data.some(
        (modelProvider) => modelProvider.configured
    );
    return (
        <ModelProviderContext.Provider
            value={{
                modelProviders: getModelProviders.data,
                configured,
                configureModelProvider: configureModelProvider.execute,
                isUpdating: configureModelProvider.isLoading,
                lastUpdated,
                error: configureModelProvider.error,
            }}
        >
            {children}
        </ModelProviderContext.Provider>
    );
}

export function useModelProviders() {
    const context = useContext(ModelProviderContext);
    if (context === undefined) {
        throw new Error(
            "useModelProvider must be used within a ModelProviderProvider"
        );
    }
    return context;
}
