import { createFromSource } from 'fumadocs-core/search/server'

import { source } from '@/lib/source'

// Static export: build the Orama index at build time and serve it as a
// static file. The client (RootProvider search type: 'static') fetches
// this and searches in the browser — no server needed on GitHub Pages.
export const revalidate = false

export const { staticGET: GET } = createFromSource(source)
