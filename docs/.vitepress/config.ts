import { defineConfig } from 'vitepress'

const enSidebar = [
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
    text: 'Behind the scenes',
    items: [
      { text: 'Built with Curiosity', link: '/built-with-curiosity' },
      { text: 'Agent vs Server', link: '/agent-vs-server' },
      { text: 'How We Built It', link: '/how-we-built-it' },
      { text: 'Workflows & CI/CD', link: '/workflow-setup' },
    ],
  },
]

const esSidebar = [
  {
    text: 'Inicio',
    items: [
      { text: 'Introducción', link: '/es/' },
      { text: 'Inicio rápido', link: '/es/inicio-rapido' },
      { text: 'Conceptos', link: '/es/conceptos-observabilidad' },
    ],
  },
  {
    text: 'Referencia',
    items: [
      { text: 'API Reference', link: '/es/referencia-api' },
      { text: 'Arquitectura', link: '/es/arquitectura' },
      { text: 'Decisiones (ADRs)', link: '/es/decisiones' },
      { text: 'Internos', link: '/es/internos' },
    ],
  },
  {
    text: 'Integraciones',
    items: [
      { text: 'Express / NestJS / Next.js', link: '/es/guia-integracion' },
    ],
  },
  {
    text: 'Detrás del proyecto',
    items: [
      { text: 'Construido con curiosidad', link: '/es/construido-con-curiosidad' },
      { text: 'Agente vs Servidor', link: '/es/agente-vs-servidor' },
      { text: 'Cómo lo construimos', link: '/es/como-lo-construimos' },
      { text: 'Workflows y CI/CD', link: '/es/configuracion-workflows' },
    ],
  },
]

export default defineConfig({
  title: 'MiniObserv',
  description: 'Self-hosted observability platform — metrics, logs, alerting and live dashboard in Go.',
  base: '/theminidog/',

  head: [
    ['link', { rel: 'icon', href: '/theminidog/favicon.svg', type: 'image/svg+xml' }],
  ],

  locales: {
    root: {
      label: 'English',
      lang: 'en-US',
      themeConfig: {
        nav: [
          { text: 'Guide', link: '/getting-started' },
          { text: 'API', link: '/api-reference' },
          { text: 'Integration', link: '/integration-guide' },
          { text: 'GitHub', link: 'https://github.com/KamerrEzz/theminidog' },
        ],
        sidebar: enSidebar,
      },
    },
    es: {
      label: 'Español',
      lang: 'es',
      link: '/es/',
      themeConfig: {
        nav: [
          { text: 'Guía', link: '/es/inicio-rapido' },
          { text: 'API', link: '/es/referencia-api' },
          { text: 'Integración', link: '/es/guia-integracion' },
          { text: 'GitHub', link: 'https://github.com/KamerrEzz/theminidog' },
        ],
        sidebar: esSidebar,
      },
    },
  },

  themeConfig: {
    logo: '/favicon.svg',

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

  ignoreDeadLinks: true,

  markdown: {
    lineNumbers: true,
  },
})
