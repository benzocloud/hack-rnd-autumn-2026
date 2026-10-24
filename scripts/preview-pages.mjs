import { createServer } from "node:http";
import { createReadStream } from "node:fs";
import { stat } from "node:fs/promises";
import { extname, resolve, sep } from "node:path";


const root = resolve("out");
const basePath = process.env.NEXT_PUBLIC_BASE_PATH ?? "/hack-rnd-autumn-2026";
const port = Number(process.env.PREVIEW_PORT || 3102);
const types = { ".html": "text/html; charset=utf-8", ".css": "text/css", ".js": "application/javascript", ".json": "application/json", ".txt": "text/plain", ".svg": "image/svg+xml", ".png": "image/png", ".ico": "image/x-icon", ".webmanifest": "application/manifest+json", ".woff2": "font/woff2" };


const server = createServer(async ( request, response ) => {
    try {
        const url = new URL(request.url, `http://127.0.0.1:${port}`);
        const pathname = decodeURIComponent(url.pathname);

        if (pathname !== basePath && !pathname.startsWith(`${basePath}/`)) {
            response.writeHead(404).end();

            return;
        }

        let file = resolve(root, `.${pathname.slice(basePath.length) || "/"}`);

        if (file !== root && !file.startsWith(`${root}${sep}`)) {
            response.writeHead(404).end();

            return;
        }

        const details = await stat(file);

        if (details.isDirectory()) {
            if (!pathname.endsWith("/")) {
                response.writeHead(301, { Location: `${url.pathname}/${url.search}` }).end();

                return;
            }

            file = resolve(file, "index.html");
        }

        await stat(file);
        response.writeHead(200, { "Content-Type": types[extname(file)] || "application/octet-stream" });

        if (request.method === "HEAD") {
            response.end();
        } else {
            createReadStream(file).pipe(response);
        }
    } catch {
        response.writeHead(404, { "Content-Type": "text/html; charset=utf-8" });
        createReadStream(resolve(root, "404.html")).pipe(response);
    }
});


server.listen(port, "127.0.0.1", () => console.log(`Предпросмотр: http://127.0.0.1:${port}${basePath}/`));
