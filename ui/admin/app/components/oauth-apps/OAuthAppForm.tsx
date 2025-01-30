import { zodResolver } from "@hookform/resolvers/zod";
import { Fragment, useEffect, useMemo } from "react";
import { useForm } from "react-hook-form";

import { OAuthAppParams } from "~/lib/model/oauthApps";
import {
	OAuthAppSpec,
	OAuthFormStep,
} from "~/lib/model/oauthApps/oauth-helpers";
import { cn } from "~/lib/utils";

import { CopyText } from "~/components/composed/CopyText";
import { ControlledInput } from "~/components/form/controlledInputs";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import {
	Accordion,
	AccordionContent,
	AccordionItem,
	AccordionTrigger,
} from "~/components/ui/accordion";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";
import { Markdown } from "~/components/ui/markdown";

type OAuthAppFormProps = {
	onSubmit: (data: OAuthAppParams) => void;
	isLoading?: boolean;
	spec: OAuthAppSpec;
};

export function OAuthAppForm({ spec, onSubmit, isLoading }: OAuthAppFormProps) {
	const fields = useMemo(() => {
		if (!spec) return [];
		return Object.entries(spec.schema.shape).map(([key]) => ({
			key: key as keyof OAuthAppParams,
		}));
	}, [spec]);

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
			<form onSubmit={handleSubmit} className="flex flex-col gap-4 p-1">
				{spec.steps.map((s, i) => (
					<Fragment key={i}>{renderStep(s)}</Fragment>
				))}

				<Button type="submit" disabled={isLoading} variant="secondary">
					{isLoading && <LoadingSpinner className="mr-2 h-4 w-4" />} Submit
				</Button>
			</form>
		</Form>
	);

	function renderStep(step: OAuthFormStep) {
		switch (step.type) {
			case "markdown":
				return <Markdown>{step.text}</Markdown>;
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
					<div className="flex justify-center">
						<CopyText
							text={step.text}
							className="w-auto max-w-fit justify-center"
						/>
					</div>
				);
			}
			case "sectionGroup": {
				return (
					<Accordion
						type="multiple"
						defaultValue={step.sections
							.filter((s) => s.defaultOpen)
							.map((_, i) => i.toString())}
					>
						{step.sections.map((section, index) => (
							<AccordionItem key={index} value={index.toString()}>
								<AccordionTrigger>{section.title}</AccordionTrigger>

								<AccordionContent
									className={cn("flex flex-col gap-1", {
										"flex-row flex-wrap justify-center gap-2":
											section.displayStepsInline,
									})}
								>
									{section.steps.map((s, i) => (
										<Fragment key={i}>{renderStep(s)}</Fragment>
									))}
								</AccordionContent>
							</AccordionItem>
						))}
					</Accordion>
				);
			}
		}
	}
}
