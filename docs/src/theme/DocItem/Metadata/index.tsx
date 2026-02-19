/**
 * Swizzled from @docusaurus/theme-classic to override canonical URLs
 * for versioned (non-latest) docs so they point to the latest version.
 *
 * React Helmet deduplicates `link[rel="canonical"]` by keeping the last
 * rendered tag.  Because DocItem renders after SiteMetadata in the
 * component tree, the canonical we emit here wins.
 */
import React, { type ReactNode, useMemo } from "react";
import { PageMetadata } from "@docusaurus/theme-common";
import {
  useDoc,
  useDocsVersion,
  useLatestVersion,
} from "@docusaurus/plugin-content-docs/client";
import Head from "@docusaurus/Head";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { PATH_REDIRECTS, escapeRegExp } from "../../../../plugins/utils";

/**
 * Build a set of valid root-relative slugs (no leading/trailing slashes)
 * from the latest version's doc paths.  The paths from Docusaurus include
 * the baseUrl prefix (e.g. "/concepts/architecture"), so we strip it.
 */
function useLatestVersionSlugs(baseUrl: string): Set<string> {
  const latestVersion = useLatestVersion(undefined);
  return useMemo(() => {
    const slugs = new Set<string>();
    for (const doc of latestVersion.docs) {
      // doc.path looks like "/concepts/architecture" (with baseUrl already applied)
      const slug = doc.path
        .replace(new RegExp(`^${escapeRegExp(baseUrl)}`), "")
        .replace(/^\//, "")
        .replace(/\/$/, "");
      slugs.add(slug);
    }
    return slugs;
  }, [latestVersion, baseUrl]);
}

/**
 * Resolve a slug to a valid canonical path in the latest version.
 *
 * Returns `null` when the slug cannot be confidently resolved, so the
 * component can leave the server-rendered canonical untouched and let the
 * postBuild canonical-urls plugin handle it (which can follow client-side
 * redirects and validate against the full build output).
 *
 * Resolution order:
 * 1. If the slug exists directly in the latest version → use it.
 * 2. If PATH_REDIRECTS has a mapping → validate the target exists, then use it.
 *    An explicit mapping to "" means "fall back to site root".
 * 3. Otherwise → return null (unresolvable).
 */
function resolveCanonicalSlug(
  slug: string,
  validSlugs: Set<string>
): string | null {
  // Case 1: slug maps directly to a real latest-version page
  if (validSlugs.has(slug)) {
    return slug;
  }

  // Case 2: explicit redirect mapping — but only if the target is valid
  if (slug in PATH_REDIRECTS) {
    const mapped = PATH_REDIRECTS[slug];
    if (mapped === "" || validSlugs.has(mapped)) {
      return mapped;
    }
  }

  // Case 3: slug cannot be resolved — leave the canonical untouched so the
  // postBuild plugin can compute a more accurate value.
  return null;
}

export default function DocItemMetadata(): ReactNode {
  const { metadata, frontMatter, assets } = useDoc();
  const version = useDocsVersion();
  const {
    siteConfig: { url: siteUrl, baseUrl, trailingSlash },
  } = useDocusaurusContext();
  const latestSlugs = useLatestVersionSlugs(baseUrl);

  // For non-latest versions (older + "next"), override the canonical URL
  // to point to the equivalent page in the latest version — but only when
  // the slug can be confidently resolved.  When resolution returns null the
  // server-rendered canonical is left intact for the postBuild canonical-urls
  // plugin to handle (it can follow client-side redirects and validate
  // against the full build output).
  let canonicalOverride: string | undefined;
  if (!version.isLast) {
    // metadata.slug is the path without base URL or version path,
    // e.g. "/concepts/architecture" for both /v0.14.0/concepts/architecture
    // and /next/concepts/architecture.
    const rawSlug = metadata.slug.replace(/^\//, "").replace(/\/$/, "");
    const resolvedSlug = resolveCanonicalSlug(rawSlug, latestSlugs);

    if (resolvedSlug !== null) {
      const trailing = trailingSlash !== false ? "/" : "";
      canonicalOverride = resolvedSlug
        ? `${siteUrl}/${resolvedSlug}${trailing}`
        : `${siteUrl}/`;
    }
  }

  return (
    <>
      <PageMetadata
        title={metadata.title}
        description={metadata.description}
        keywords={frontMatter.keywords}
        image={assets.image ?? frontMatter.image}
      />
      {canonicalOverride && (
        <Head>
          <link rel="canonical" href={canonicalOverride} />
          <meta property="og:url" content={canonicalOverride} />
        </Head>
      )}
    </>
  );
}
