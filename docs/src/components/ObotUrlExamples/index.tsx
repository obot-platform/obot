import React, { type ReactNode } from "react";
import { ObotUrlExamplesContext } from "../../lib/obotUrl";

export default function ObotUrlExamples({
  children,
}: {
  children: ReactNode;
}): ReactNode {
  return (
    <ObotUrlExamplesContext.Provider value>
      {children}
    </ObotUrlExamplesContext.Provider>
  );
}
