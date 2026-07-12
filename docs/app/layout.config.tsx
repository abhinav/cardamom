import type { BaseLayoutProps } from 'fumadocs-ui/layouts/shared'

export const baseOptions: BaseLayoutProps = {
  nav: {
    title: (
      <span
        className="font-bold"
        style={{
          fontFamily: 'var(--font-orbitron), sans-serif',
          textTransform: 'uppercase',
          letterSpacing: '0.2em',
        }}
      >
        clu
      </span>
    ),
  },
  links: [
    {
      text: 'Docs',
      url: '/docs',
      active: 'nested-url',
    },
    {
      text: 'GitHub',
      url: 'https://github.com/arjia-labs/clu',
      active: 'none',
    },
  ],
}
