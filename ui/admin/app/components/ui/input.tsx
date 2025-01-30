import { type VariantProps, cva } from "class-variance-authority";
import { EyeIcon, EyeOffIcon } from "lucide-react";
import * as React from "react";

import { cn } from "~/lib/utils";

import { buttonVariants } from "~/components/ui/button";

const InputReset =
	"w-full p-3 bg-transparent border-none focus-visible:border-none focus-visible:outline-none rounded-full";

const inputVariants = cva(
	cn(
		"flex h-9 w-full items-center rounded-md border border-input bg-background text-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-50 has-[:focus-visible]:ring-1 has-[:focus-visible]:ring-ring"
	),
	{
		variants: {
			variant: {
				default: "",
				ghost:
					"mb-0 cursor-pointer border-transparent px-0 font-bold shadow-none outline-none hover:border-primary has-[:focus-visible]:border-primary",
			},
		},
		defaultVariants: {
			variant: "default",
		},
	}
);

export interface InputProps
	extends React.InputHTMLAttributes<HTMLInputElement>,
		VariantProps<typeof inputVariants> {
	disableToggle?: boolean;
}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
	({ className, variant, type, disableToggle = false, ...props }, ref) => {
		const isPassword = type === "password";
		const [isVisible, setIsVisible] = React.useState(false);

		const internalType = isPassword
			? isVisible && !disableToggle
				? "text"
				: "password"
			: type;

		const toggleVisible = React.useCallback(
			() => setIsVisible((prev) => !prev),
			[]
		);

		return (
			<div className={cn(inputVariants({ variant, className }))}>
				<input
					type={internalType}
					data-1p-ignore={!isPassword}
					className={InputReset}
					ref={ref}
					{...props}
				/>

				{isPassword && !disableToggle && (
					<button
						type="button"
						onClick={toggleVisible}
						className={buttonVariants({
							variant: "ghost",
							size: "icon",
							shape: "default",
							className: "!h-full min-h-full rounded-s-none",
						})}
					>
						{!isVisible ? <EyeIcon /> : <EyeOffIcon />}
					</button>
				)}
			</div>
		);
	}
);
Input.displayName = "Input";

export { Input, inputVariants };
