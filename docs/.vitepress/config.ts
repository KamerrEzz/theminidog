import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'MiniObserv',
  description: 'Self-hosted observability platform — metrics, logs, alerting and live dashboard in Go.',
  base: '/theminidog/',

  head: [
    ['link', { rel: 'icon', href: '/theminidog/favicon.svg', type: 'image/svg+xml' }],
  ],

  themeConfig: {
    logo: '/favicon.svg',

    nav: [
      { text: 'Guide', link: '/getting-started' },
      { text: 'API', link: '/api-reference' },
      { text: 'Integration', link: '/integration-guide' },
      {
        text: 'GitHub',
        link: 'https://github.com/KamerrEzz/theminidog',
      },
    ],

    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Introduction', link: '/' },
          { text: 'Quick Start', link: '/getting-started' },
          { text: 'Concepts', link: '/observability-concepts' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'API Reference', link: '/api-reference' },
          { text: 'Architecture', link: '/architecture' },
          { text: 'Decisions (ADRs)', link: '/decisions' },
          { text: 'Internals', link: '/internals' },
        ],
      },
      {
        text: 'Integrations',
        items: [
          { text: 'Express / NestJS / Next.js', link: '/integration-guide' },
        ],
      },
      {
        text: 'Español',
        items: [
          { text: 'Guía de integración', link: '/es/guia-integracion' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/KamerrEzz/theminidog' },
    ],

    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Built by <a href="https://github.com/KamerrEzz">KamerrEzz</a>',
    },

    editLink: {
      pattern: 'https://github.com/KamerrEzz/theminidog/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },

    search: {
      provider: 'local',
    },
  },

  markdown: {
    lineNumbers: true,
  },
})
