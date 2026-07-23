import React, { type ReactNode } from "react";
import OriginalCodeBlock from "@theme-original/CodeBlock";
import type { Props } from "@theme/CodeBlock";
import {
  DEFAULT_OBOT_URL,
  useObotUrl,
  useObotUrlExamples,
} from "../../lib/obotUrl";

function ConfigurableCodeBlock({ children, ...props }: Props): ReactNode {
  const obotUrl = useObotUrl();
  const content =
    typeof children === "string"
      ? children.replaceAll(DEFAULT_OBOT_URL, obotUrl)
      : children;

  return <OriginalCodeBlock {...props}>{content}</OriginalCodeBlock>;
}

export default function CodeBlock(props: Props): ReactNode {
  return useObotUrlExamples() ? (
    <ConfigurableCodeBlock {...props} />
  ) : (
    <OriginalCodeBlock {...props} />
  );
}
