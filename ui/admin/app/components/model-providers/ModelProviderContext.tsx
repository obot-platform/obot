import {
    ReactNode,
    createContext,
    useCallback,
    useContext,
    useState,
} from "react";
import useSWR, { mutate } from "swr";

import { ModelProvider } from "~/lib/model/modelProviders";
import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";

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

export function ModelProviderProvider({ children }: { children: ReactNode }) {
    const getModelProviders = useSWR(
        ModelProviderApiService.getModelProviders.key(),
        () => ModelProviderApiService.getModelProviders(),
        { fallbackData: [] }
    );

    const [lastUpdated, setLastSaved] = useState<Date>();

    const handleConfigureModelProvider = useCallback(
        (modelProviderId: string, values: Record<string, string>) =>
            ModelProviderApiService.configureModelProviderById(
                modelProviderId,
                values
            )
                .then(() => {
                    getModelProviders.mutate();
                    mutate(ModelProviderApiService.getModelProviders.key());
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
