import { ReactNode, createContext, useContext } from "react";
import useSWR from "swr";

import { ModelProvider } from "~/lib/model/modelProviders";
import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";

interface ModelProviderContextType {
    modelProviderConfigured: boolean;
    modelProviders: ModelProvider[];
}

const ModelProviderContext = createContext<
    ModelProviderContextType | undefined
>(undefined);

export function ModelProviderProvider({ children }: { children: ReactNode }) {
    const { data: modelProviders } = useSWR(
        ModelProviderApiService.getModelProviders.key(),
        () => ModelProviderApiService.getModelProviders()
    );
    const modelProviderConfigured =
        modelProviders?.some((modelProvider) => modelProvider.configured) ??
        false;
    return (
        <ModelProviderContext.Provider
            value={{
                modelProviderConfigured,
                modelProviders: modelProviders ?? [],
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
