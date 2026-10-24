import React, { type ReactNode } from "react";
import Content from "@theme-original/DocItem/Content";
import { useDoc } from "@docusaurus/plugin-content-docs/client";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import type { Props } from "@theme/DocItem/Content";

export default function DocItemContent(props: Props): ReactNode {
  const { metadata, frontMatter } = useDoc();
  const {
    siteConfig: { baseUrl },
  } = useDocusaurusContext();
  const version = metadata.version === "current" ? "next" : metadata.version;
  const route = metadata.slug.replace(/^\/+|\/+$/g, "") || "index";
  return (
    <>
      {!metadata.unlisted && !frontMatter.draft && (
        <p>
          <a href={`${baseUrl}${version}/${route}.md`}>View as Markdown</a>
        </p>
      )}
      <Content {...props} />
    </>
  );
}
