import { createMDX } from 'fumadocs-mdx/next'
import { fileURLToPath } from 'node:url'
import { dirname } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))

const withMDX = createMDX()

// GitHub Pages project sites are served from a sub-path
// (https://<org>.github.io/<repo>/). The deploy workflow sets
// PAGES_BASE_PATH to "/clu"; locally it's empty so `next dev` /
// `next build` work at the root.
const basePath = process.env.PAGES_BASE_PATH || ''

/** @type {import('next').NextConfig} */
const config = {
  // Emit a fully static site into ./out for GitHub Pages.
  output: 'export',
  basePath,
  // The Next image optimizer needs a server; static export can't use it.
  images: { unoptimized: true },
  // Surface the base path to the client so the static search index is
  // fetched from the correct sub-path.
  env: { NEXT_PUBLIC_BASE_PATH: basePath },
  turbopack: {
    root: __dirname,
  },
}

export default withMDX(config)
