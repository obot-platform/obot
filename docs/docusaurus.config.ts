import type * as Preset from "@docusaurus/preset-classic";
import type { Config } from "@docusaurus/types";
import { themes as prismThemes } from "prism-react-renderer";

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

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

  markdown: {
    hooks: {
      onBrokenMarkdownLinks: "warn",
    },
  },

  i18n: {
    defaultLocale: "en",
    locales: ["en"],
  },

  plugins: [
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

          // Versioning configuration
          lastVersion: "v0.15.0",
          versions: {
            "v0.14.0": { label: "v0.14.0", banner: "none", path: "v0.14.0" },
            "v0.13.0": { label: "v0.13.0", banner: "none", path: "v0.13.0" },
          },
        },
        theme: {
          customCss: "./src/css/custom.css",
        },
        blog: false,
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
