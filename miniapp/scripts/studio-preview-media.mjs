import { createReadStream, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'

// Serve existing repository videos only during the local preview. Media never
// enters the mini-program package; range responses support video seeking.
export function studioPreviewMedia() {
  const directory = fileURLToPath(new URL('../../website-react/public/assets/videos/', import.meta.url))
  return {
    name: 'studio-preview-media',
    configureServer(server) {
      if (process.env.VITE_UI_PREVIEW !== 'true') return
      const serveMedia = (req, res) => {
        const filename = (req.url || '').split('?')[0].replace(/^\//, '')
        if (!/^(?:laohan-\d{2}\.mp4|posters\/laohan-\d{2}\.jpg)$/.test(filename)) { res.statusCode = 404; res.end(); return }
        try {
          const path = join(directory, filename)
          const { size } = statSync(path)
          const range = req.headers.range?.match(/^bytes=(\d+)-(\d*)$/)
          const start = range ? Number(range[1]) : 0
          const end = range && range[2] ? Math.min(Number(range[2]), size - 1) : size - 1
          if (start > end || start >= size) { res.writeHead(416, { 'Content-Range': `bytes */${size}` }); res.end(); return }
          res.writeHead(range ? 206 : 200, { 'Content-Type': filename.endsWith('.jpg') ? 'image/jpeg' : 'video/mp4', 'Content-Length': end - start + 1, 'Accept-Ranges': 'bytes', ...(range ? { 'Content-Range': `bytes ${start}-${end}/${size}` } : {}) })
          if (req.method === 'HEAD') { res.end(); return }
          const stream = createReadStream(path, { start, end })
          stream.on('error', () => res.destroy())
          res.on('close', () => stream.destroy())
          stream.pipe(res)
        } catch { res.statusCode = 404; res.end() }
      }
      server.middlewares.use('/__studio-media', serveMedia)
      server.middlewares.use('/static/studio-preview', serveMedia)
    },
  }
}
