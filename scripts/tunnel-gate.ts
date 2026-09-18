import { createServer, connect, type Socket } from "node:net";

export async function startTunnelGate(options: {
    secret: string;
    port?: number;
    originPort: number;
}) {
    if (!/^[A-Za-z0-9_-]{43}$/.test(options.secret)) throw new Error("Invalid tunnel capability");
    const path = `/${options.secret}`;
    const sockets = new Set<Socket>();
    let upgraded = 0;
    const server = createServer((socket) => {
        socket.setNoDelay(true);
        sockets.add(socket);
        socket.on("close", () => sockets.delete(socket));
        socket.on("error", () => socket.destroy());
        socket.setTimeout(10_000, () => socket.destroy());
        let pending = Buffer.alloc(0);
        function reply(status: string, body = "") {
            socket.end(`HTTP/1.1 ${status}\r\nContent-Type: text/plain\r\nContent-Length: ${Buffer.byteLength(body)}\r\nCache-Control: no-store\r\nConnection: close\r\n\r\n${body}`);
        }
        function handshake(chunk: Buffer) {
            pending = Buffer.concat([pending, chunk]);
            const end = pending.indexOf("\r\n\r\n");
            if (end < 0) {
                if (pending.length > 16_384) socket.destroy();
                return;
            }
            socket.removeListener("data", handshake);
            if (end > 16_384) { socket.destroy(); return; }
            const header = pending.subarray(0, end).toString("latin1");
            const lines = header.split("\r\n");
            const request = lines.shift()!.match(/^GET ([^ ]+) HTTP\/1\.[01]$/);
            if (!request || request[1] !== path) { reply("404 Not Found"); return; }
            if (lines.some((line) => !/^[!#$%&'*+.^_`|~0-9A-Za-z-]+:[\x09\x20-\x7e\x80-\xff]*$/.test(line))) {
                reply("400 Bad Request"); return;
            }
            const isUpgrade = lines.some((line) => /^Upgrade:\s*websocket\s*$/i.test(line)) &&
                lines.some((line) => /^Connection:/i.test(line) && line.slice(line.indexOf(":") + 1).split(",").some((part) => part.trim().toLowerCase() === "upgrade"));
            if (!isUpgrade) { reply("200 OK", "StageWand tunnel ready\n"); return; }
            if (upgraded >= 32) { reply("503 Service Unavailable"); return; }
            upgraded++;
            socket.pause();
            socket.setTimeout(0);
            const origin = connect({ host: "127.0.0.1", port: options.originPort });
            origin.setNoDelay(true);
            sockets.add(origin);
            let cleaned = false;
            const timer = setTimeout(cleanup, 10_000);
            function cleanup() {
                if (cleaned) return;
                cleaned = true;
                clearTimeout(timer);
                upgraded--;
                sockets.delete(origin);
                origin.destroy();
                socket.destroy();
            }
            socket.on("close", cleanup);
            socket.on("error", cleanup);
            origin.on("close", cleanup);
            origin.on("error", cleanup);
            origin.once("data", () => clearTimeout(timer));
            origin.once("connect", () => {
                if (cleaned) return;
                origin.write(`GET / HTTP/1.1\r\n${lines.join("\r\n")}\r\n\r\n`, "latin1");
                const head = pending.subarray(end + 4);
                if (head.length) origin.write(head);
                pending = Buffer.alloc(0);
                origin.pipe(socket);
                socket.pipe(origin);
                socket.resume();
            });
        }
        socket.on("data", handshake);
    });
    await new Promise<void>((resolve, reject) => {
        server.once("error", reject);
        server.listen(options.port ?? 18788, "127.0.0.1", () => {
            server.removeListener("error", reject);
            resolve();
        });
    });
    const address = server.address();
    if (!address || typeof address === "string") throw new Error("Gate did not listen");
    return {
        port: address.port,
        close: () => new Promise<void>((resolve) => {
            server.close(() => resolve());
            for (const socket of sockets) socket.destroy();
        }),
    };
}
