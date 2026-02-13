import { themes as prismThemes } from "prism-react-renderer";
import type { Config } from "@docusaurus/types";
import type * as Preset from "@docusaurus/preset-classic";
import versions from "./versions.json";

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

// First version in versions.json is the latest, rest are older versions
// If versions.json is empty, fall back to the "current" version label.
const latestVersion = versions[0] ?? "current";
// olderVersions may legitimately be empty (when there is only one or zero versions).
// This is safe: the subsequent .map calls will just operate on an empty array.
const olderVersions = versions.slice(1);

// Generate version config for older versions (latest is served at root)
// "unmaintained" banner shows a warning that this is an older version with link to latest
const versionsConfig = Object.fromEntries(
  olderVersions.map((version) => [
    version,
    { label: version, banner: "unmaintained" as const, path: version },
  ])
);

// Generate sitemap ignore patterns for older versions
const sitemapIgnorePatterns = olderVersions.map((version) => `/${version}/**`);

const config: Config = {
  title: "Obot Docs",
  tagline: "",
  favicon: "img/favicon.ico",
  url: "https://docs.obot.ai",
  baseUrl: "/",
  trailingSlash: true,
  organizationName: "obot-platform",
  projectName: "obot",
  onBrokenLinks: "throw",
  onBrokenMarkdownLinks: "warn",

  i18n: {
    defaultLocale: "en",
    locales: ["en"],
  },

  plugins: [
    // Custom plugin to rewrite canonical URLs in versioned docs to point to latest
    "./plugins/canonical-urls.ts",
    // Custom plugin to inject JSON-LD structured data into built HTML pages
    "./plugins/structured-data.ts",
    [
      "@docusaurus/plugin-client-redirects",
      {
        redirects: [
          {
            from: "/concepts/admin/mcp-server-catalogs",
            to: "/configuration/mcp-server-gitops",
          },
          {
            from: "/concepts/mcp-gateway/overview",
            to: "/concepts/mcp-gateway",
          },
          {
            from: "/installation/general",
            to: "/installation/overview",
          },
          {
            from: "/tutorials/knowledge-assistant",
            to: "/",
          },
        ],
      },
    ],
  ],

  presets: [
    [
      "classic",
      {
        docs: {
          sidebarPath: "./sidebars.ts",
          editUrl: "https://github.com/obot-platform/obot/tree/main/docs",
          routeBasePath: "/", // Serve the docs at the site's root

          // Versioning configuration - dynamically generated from versions.json
          lastVersion: latestVersion,
          versions: versionsConfig,
        },
        theme: {
          customCss: "./src/css/custom.css",
        },
        blog: false,
        sitemap: {
          // Exclude older versioned docs from sitemap - only index the latest version
          // Dynamically generated from versions.json
          ignorePatterns: sitemapIgnorePatterns,
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    // Replace with your project's social card
    image: "img/obot-logo-blue-black-text.svg",
    navbar: {
      logo: {
        alt: "Obot Logo",
        src: "img/obot-logo-blue-black-text.svg",
        srcDark: "img/obot-logo-blue-white-text.svg",
      },
      items: [
        {
          type: "docsVersionDropdown",
          position: "left",
          dropdownActiveClassDisabled: true,
        },
        {
          href: "https://github.com/obot-platform/obot",
          label: "GitHub",
          position: "right",
        },
        {
          href: "https://discord.gg/9sSf4UyAMC",
          label: "Discord",
          position: "right",
        },
      ],
    },
    footer: {
      style: "dark",
      links: [
        {
          label: "GitHub",
          to: "https://github.com/obot-platform/obot",
        },
        {
          label: "Discord",
          to: "https://discord.gg/9sSf4UyAMC",
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Obot AI, Inc`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.vsDark,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
