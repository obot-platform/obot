import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

const config: Config = {
  title: 'Otto8 Docs',
  tagline: '',
  favicon: 'img/favicon.ico',
  url: 'https://docs.otto8.ai',
  baseUrl: '/',
  organizationName: 'otto8-ai',
  projectName: 'otto8',
  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'warn',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          editUrl: 'https://github.com/otto8-ai/otto8/tree/main/docs',
          routeBasePath: "/", // Serve the docs at the site's root
        },
        theme: {
          customCss: './src/css/custom.css',
        },
        blog: false,
      } satisfies Preset.Options,
    ],
  ],

  plugins: [
    require.resolve('./src/plugins/fetch-snippets'),
  ],

  themeConfig: {
    // Replace with your project's social card
    image: 'img/otto8-logo-blue-black-text.svg',
    navbar: {
      logo: {
        alt: 'Otto8 Logo',
        src: 'img/otto8-logo-blue-black-text.svg',
        srcDark: 'img/otto8-logo-blue-white-text.svg',
      },
      items: [
        {
          href: "https://github.com/otto8-ai/otto8",
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
      style: 'dark',
      links: [
        {
          label: "GitHub",
          to: "https://github.com/otto8-ai/otto8",
        },
        {
          label: "Discord",
          to: "https://discord.gg/9sSf4UyAMC",
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Acorn Labs, Inc`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
