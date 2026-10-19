import { cpSync, mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const source = join(here, '..', 'node_modules', 'pdfjs-dist')
const target = join(here, '..', 'public', 'pdfjs')

mkdirSync(target, { recursive: true })
for (const directory of ['cmaps', 'standard_fonts', 'wasm', 'iccs']) {
  cpSync(join(source, directory), join(target, directory), {
    recursive: true,
    force: true
  })
}

console.log('Copied pdf.js assets to public/pdfjs')
