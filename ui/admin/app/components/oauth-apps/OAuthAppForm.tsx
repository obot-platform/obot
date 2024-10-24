import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useMemo } from "react";
import { useForm } from "react-hook-form";
import Markdown from "react-markdown";
import rehypeExternalLinks from "rehype-external-links";

import { OAuthAppParams } from "~/lib/model/oauthApps";
import {
    OAuthFormStep,
    OAuthProvider,
} from "~/lib/model/oauthApps/oauth-helpers";
import { cn } from "~/lib/utils";

import { CopyText } from "~/components/composed/CopyText";
import { ControlledInput } from "~/components/form/controlledInputs";
import { CustomMarkdownComponents } from "~/components/react-markdown";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";
import { useOAuthAppInfo } from "~/hooks/oauthApps/useOAuthApps";

import {
    Accordion,
    AccordionContent,
    AccordionItem,
    AccordionTrigger,
} from "../ui/accordion";

type OAuthAppFormProps = {
    type: OAuthProvider;
    onSubmit: (data: OAuthAppParams) => void;
    isLoading?: boolean;
};

export function OAuthAppForm({ type, onSubmit, isLoading }: OAuthAppFormProps) {
    const spec = useOAuthAppInfo(type);

    const fields = useMemo(() => {
        return Object.entries(spec.schema.shape).map(([key]) => ({
            key: key as keyof OAuthAppParams,
        }));
    }, [spec.schema]);

    const defaultValues = useMemo(() => {
        return fields.reduce((acc, { key }) => {
            acc[key] = "";
            return acc;
        }, {} as OAuthAppParams);
    }, [fields]);

    const form = useForm({
        defaultValues,
        resolver: zodResolver(spec.schema),
    });

    useEffect(() => {
        form.reset(defaultValues);
    }, [defaultValues, form]);

    const handleSubmit = form.handleSubmit(onSubmit);

    return (
        <Form {...form}>
            <form onSubmit={handleSubmit} className="flex flex-col gap-4">
                {spec.steps.map(renderStep)}

                <Button type="submit" disabled={isLoading}>
                    {isLoading && <LoadingSpinner className="w-4 h-4 mr-2" />}{" "}
                    Submit
                </Button>
            </form>
        </Form>
    );

    function renderStep(step: OAuthFormStep) {
        switch (step.type) {
            case "markdown":
                return (
                    <Markdown
                        className={cn(
                            "flex-auto max-w-full prose overflow-x-auto dark:prose-invert prose-pre:whitespace-pre-wrap prose-pre:break-words prose-thead:text-left prose-img:rounded-xl prose-img:shadow-lg break-words"
                        )}
                        components={CustomMarkdownComponents}
                        rehypePlugins={[
                            [rehypeExternalLinks, { target: "_blank" }],
                        ]}
                    >
                        {step.text}
                    </Markdown>
                );
            case "input": {
                return (
                    <ControlledInput
                        key={step.input}
                        name={step.input as keyof OAuthAppParams}
                        label={step.label}
                        control={form.control}
                        type={step.inputType}
                    />
                );
            }
            case "copy": {
                return (
                    <div className="w-full flex justify-center">
                        <CopyText
                            text={step.text}
                            className="mx-8 w-auto max-w-fit justify-center"
                        />
                    </div>
                );
            }
            case "sectionGroup": {
                return (
                    <Accordion type="single" collapsible>
                        {step.sections.map((section, index) => (
                            <AccordionItem key={index} value={index.toString()}>
                                <AccordionTrigger>
                                    {section.title}
                                </AccordionTrigger>
                                <AccordionContent>
                                    {section.steps.map(renderStep)}
                                </AccordionContent>
                            </AccordionItem>
                        ))}
                    </Accordion>
                );
            }
        }
    }
}
