import fs from 'node:fs';
import path from 'node:path';
import type * as Preset from '@docusaurus/preset-classic';
import type { Config } from '@docusaurus/types';
import { themes as prismThemes } from 'prism-react-renderer';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

/**
 * Versioning policy — see VERSIONING.md for the full runbook.
 *
 * A snapshot is cut only when a release changes documented behaviour — a
 * changed default, a rename, a removal, a deprecation — or when it fixes a
 * gap in an already-released version's docs. Purely additive releases get an
 * `_0.17+_` marker in docs/ instead, so expect this list to grow slower than
 * the release count.
 *
 * Snapshots are MAJOR.MINOR ("0.16"), never per patch (rabbitwrap has not cut
 * a patch release yet, but the rule stands: a patch that changes documented
 * behaviour is edited into the existing snapshot in place).
 *
 * Only the newest `maxLiveVersions` snapshots are built. Older ones stay in
 * git (readable at their tag) but are dropped from the site so build time and
 * search index size stay flat as releases accumulate.
 *
 * The window lives in versions.config.json because cut-version.mjs needs the
 * same number to decide which snapshots it is about to push out of the live
 * set. Duplicating it meant the script could warn about a different window
 * than the one the site actually builds.
 */
const MAX_LIVE_VERSIONS: number = JSON.parse(
  fs.readFileSync(path.resolve(__dirname, 'versions.config.json'), 'utf8'),
).maxLiveVersions;

/** Shared so the literal paths in headTags cannot drift from `baseUrl`. */
const BASE_URL = '/rabbitwrap/';

const versionsFile = path.resolve(__dirname, 'versions.json');
const allVersions: string[] = fs.existsSync(versionsFile)
  ? JSON.parse(fs.readFileSync(versionsFile, 'utf8'))
  : [];

/**
 * `DOCS_FAST_BUILD=true` builds only the in-progress docs plus the newest
 * live version. Used by `start` and by the PR build check, where rebuilding
 * every historical version is wasted work. Production deploys build the
 * full live set.
 */
const fastBuild = process.env.DOCS_FAST_BUILD === 'true';
const liveVersions = allVersions.slice(0, MAX_LIVE_VERSIONS);
const includedVersions = fastBuild
  ? ['current', ...allVersions.slice(0, 1)]
  : ['current', ...liveVersions];

const config: Config = {
  title: 'rabbitwrap',
  tagline: 'A production-ready RabbitMQ client wrapper for Go',
  // .ico carries 16/32/48 frames for the browsers that ignore SVG favicons;
  // the SVG below is preferred by everything current and stays sharp on hidpi.
  favicon: 'img/favicon.ico',

  headTags: [
    {
      tagName: 'link',
      attributes: {
        rel: 'icon',
        type: 'image/svg+xml',
        href: `${BASE_URL}img/favicon.svg`,
      },
    },
    {
      tagName: 'link',
      attributes: {
        rel: 'apple-touch-icon',
        href: `${BASE_URL}img/apple-touch-icon.png`,
      },
    },
  ],

  future: {
    v4: true,
    // Rspack/SWC build pipeline — matters here because build time scales with
    // the number of versioned doc trees.
    faster: true,
  },

  url: 'https://kartikrocks.github.io',
  baseUrl: BASE_URL,

  organizationName: 'KARTIKrocks',
  projectName: 'rabbitwrap',
  trailingSlash: false,

  onBrokenLinks: 'throw',

  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'throw',
    },
  },

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
          editUrl:
            'https://github.com/KARTIKrocks/rabbitwrap/tree/main/website/',
          // `current` is the working copy on main — it documents unreleased
          // changes and is served at /docs/next/. The newest snapshot in
          // versions.json is what /docs/ serves, so the default reader always
          // lands on released behaviour.
          versions: {
            current: {
              label: 'Next (unreleased)',
              path: 'next',
              banner: 'unreleased',
            },
          },
          onlyIncludeVersions: includedVersions,
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
        sitemap: {
          lastmod: 'date',
          changefreq: 'weekly',
          priority: 0.5,
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    image: 'img/rabbitwrap-social-card.png',
    colorMode: {
      defaultMode: 'light',
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'rabbitwrap',
      logo: {
        alt: 'rabbitwrap',
        src: 'img/logo.svg',
        // The mark uses the same primary token as the theme, so it needs the
        // dark-mode value (#2dd4bf) on a slate ground the way every other
        // primary-colored element does.
        srcDark: 'img/logo-dark.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docsSidebar',
          position: 'left',
          label: 'Docs',
        },
        {
          type: 'docsVersionDropdown',
          position: 'right',
          dropdownItemsAfter: [
            {
              href: 'https://github.com/KARTIKrocks/rabbitwrap/releases',
              label: 'All releases',
            },
          ],
        },
        {
          href: 'https://pkg.go.dev/github.com/KARTIKrocks/rabbitwrap',
          label: 'API Reference',
          position: 'right',
        },
        {
          href: 'https://github.com/KARTIKrocks/rabbitwrap',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'light',
      links: [
        {
          title: 'Docs',
          items: [
            { label: 'Getting Started', to: '/docs/getting-started' },
            { label: 'Connection', to: '/docs/connection' },
            { label: 'Topology', to: '/docs/topology' },
          ],
        },
        {
          title: 'Reference',
          items: [
            {
              label: 'pkg.go.dev',
              href: 'https://pkg.go.dev/github.com/KARTIKrocks/rabbitwrap',
            },
            {
              label: 'Changelog',
              href: 'https://github.com/KARTIKrocks/rabbitwrap/blob/main/CHANGELOG.md',
            },
            {
              label: 'Releases',
              href: 'https://github.com/KARTIKrocks/rabbitwrap/releases',
            },
          ],
        },
        {
          title: 'More',
          items: [
            {
              label: 'GitHub',
              href: 'https://github.com/KARTIKrocks/rabbitwrap',
            },
            {
              label: 'Issues',
              href: 'https://github.com/KARTIKrocks/rabbitwrap/issues',
            },
            {
              label: 'Contributing',
              href: 'https://github.com/KARTIKrocks/rabbitwrap/blob/main/CONTRIBUTING.md',
            },
          ],
        },
      ],
      copyright: `rabbitwrap is open source under the MIT License. Copyright © ${new Date().getFullYear()}.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['go', 'bash', 'json', 'yaml'],
    },
    // Algolia DocSearch — apply at https://docsearch.algolia.com/apply/
    // Uncomment and fill in the credentials once the application is approved.
    // algolia: {
    //   appId: 'YOUR_APP_ID',
    //   apiKey: 'YOUR_SEARCH_API_KEY',
    //   indexName: 'rabbitwrap',
    //   contextualSearch: true,
    // },
  } satisfies Preset.ThemeConfig,
};

export default config;
