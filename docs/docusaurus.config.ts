import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

const config: Config = {
  title: 'Obot Docs',
  tagline: '',
  favicon: 'img/favicon.ico',
  url: 'https://docs.obot.ai',
  baseUrl: '/',
  organizationName: 'obot-platform',
  projectName: 'obot',
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
          editUrl: 'https://github.com/obot-platform/obot/tree/main/docs',
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
    image: 'img/obot-logo-blue-black-text.svg',
    navbar: {
      logo: {
        alt: 'Obot Logo',
        src: 'img/obot-logo-blue-black-text.svg',
        srcDark: 'img/obot-logo-blue-white-text.svg',
      },
      items: [
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
      style: 'dark',
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
      copyright: `Copyright © ${new Date().getFullYear()} Acorn Labs, Inc`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
