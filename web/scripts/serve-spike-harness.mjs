import { createReadStream, existsSync, statSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, join, normalize } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('../tests/spikes/harness/.output/public/', import.meta.url))
const types = { '.css': 'text/css', '.html': 'text/html', '.js': 'text/javascript', '.json': 'application/json', '.svg': 'image/svg+xml' }
createServer((request, response) => {
  const path = normalize(decodeURIComponent(new URL(request.url || '/', 'http://localhost').pathname)).replace(/^\.\.(\/|\\|$)/, '')
  let file = join(root, path === '/' ? 'index.html' : path)
  if (!existsSync(file) || statSync(file).isDirectory()) file = join(root, 'index.html')
  response.writeHead(200, { 'Content-Type': types[extname(file)] || 'application/octet-stream' })
  createReadStream(file).pipe(response)
}).listen(4182, '127.0.0.1')
