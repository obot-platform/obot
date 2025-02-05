import { Slot } from "@radix-ui/react-slot";
import { type VariantProps, cva } from "class-variance-authority";
import { Loader2 } from "lucide-react";
import * as React from "react";

import { cn } from "~/lib/utils";

const buttonVariants = cva(
	"inline-flex items-center justify-center gap-2 whitespace-nowrap text-sm font-medium transition-colors hover:shadow-inner focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
	{
		variants: {
			variant: {
				default:
					"bg-primary text-primary-foreground shadow hover:bg-primary/80 focus-visible:ring-foreground",
				destructive:
					"bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/80",
				outline:
					"border border-input bg-background shadow-sm hover:bg-muted/80",
				secondary:
					"bg-secondary text-secondary-foreground shadow-sm hover:bg-secondary/80",
				ghost: "hover:bg-secondary hover:text-secondary-foreground",
				"ghost-primary": "text-primary hover:bg-primary/10",
				accent: "bg-accent text-accent-foreground shadow-sm hover:bg-accent/80",
				link: "text-primary underline-offset-4 shadow-none hover:text-primary/70 hover:underline hover:shadow-none",
				warning:
					"bg-warning text-primary-foreground shadow-sm hover:bg-warning/80",
			},
			size: {
				none: "",
				link: "p-0",
				"link-sm": "p-0 text-xs",
				default: "h-9 px-4 py-2",
				badge: "px-2 py-0.5 text-xs",
				sm: "h-8 px-3 text-xs",
				lg: "h-10 px-8",
				icon: "h-9 min-h-9 w-9 min-w-9 [&_svg]:size-[1.375rem]",
				"icon-sm": "h-8 min-h-8 w-8 min-w-8 [&_svg]:size-[1.125rem]",
				"icon-xl":
					"h-20 min-h-20 w-20 min-w-20 [&_img]:size-[6rem] [&_svg]:size-[6rem]",
			},
			shape: {
				none: "",
				default: "rounded-md",
				pill: "rounded-full",
				"input-end": "rounded-l-none rounded-r-md",
			},
		},
		defaultVariants: {
			variant: "default",
			size: "default",
			shape: "pill",
		},
	}
);

export type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> &
	VariantProps<typeof buttonVariants> & {
		asChild?: boolean;
		loading?: boolean;
		startContent?: React.ReactNode;
		endContent?: React.ReactNode;
		classNames?: {
			content?: string;
		};
	};

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
	(
		{
			className,
			variant,
			size,
			shape,
			asChild = false,
			loading = false,
			startContent,
			endContent,
			children,
			classNames,
			...props
		},
		ref
	) => {
		const Comp = asChild ? Slot : "button";

		return (
			<Comp
				className={cn(buttonVariants({ variant, size, shape, className }))}
				ref={ref}
				{...props}
			>
				{getContent()}
			</Comp>
		);

		function getContent() {
			if ((size === "icon" || size === "icon-sm") && loading)
				return <Loader2 className="animate-spin" />;

			return loading ? (
				<div className="flex items-center gap-2">
					<Loader2 className="mr-2 animate-spin" />
					{children}
					{endContent}
				</div>
			) : (
				<div className={cn("flex items-center gap-2", classNames?.content)}>
					{startContent}
					{children}
					{endContent}
				</div>
			);
		}
	}
);
Button.displayName = "Button";

export { Button, buttonVariants };
