import { ComponentProps } from "react";

import { Animate } from "~/components/ui/animate";
import { SlideInOutProps, useSlideInOut } from "~/hooks/animate/useSlideInOut";

export function SlideInOut({
	direction = "left",
	translatePercent = 50,
	...restProps
}: SlideInOutProps & ComponentProps<typeof Animate.div>) {
	const animateProps = useSlideInOut({ direction, translatePercent });

	return <Animate.div {...animateProps} {...restProps} />;
}
