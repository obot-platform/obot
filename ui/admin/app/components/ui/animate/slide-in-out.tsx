import { ComponentProps, forwardRef } from "react";

import { Animate } from "~/components/ui/animate";
import { SlideInOutProps, useSlideInOut } from "~/hooks/animate/useSlideInOut";

export const SlideInOut = forwardRef<
	HTMLDivElement,
	SlideInOutProps & ComponentProps<typeof Animate.div>
>(({ direction, translatePercent, ...restProps }, ref) => {
	const animateProps = useSlideInOut({ direction, translatePercent });

	return <Animate.div {...animateProps} {...restProps} ref={ref} />;
});

SlideInOut.displayName = "SlideInOut";
