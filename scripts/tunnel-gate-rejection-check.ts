import { strict as assert } from "node:assert";
import { EventEmitter } from "node:events";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";

// Execute the actual handler with controlled sockets and time: no loopback
// permissions needed, and flushing can be stalled independently of inactivity.
class Client extends EventEmitter {
    destroyed = false;
    response = "";
    flushed?: () => void;
    setNoDelay() {}
    setTimeout() {} // Incoming trickles can indefinitely refresh inactivity.
    end(response: string, flushed?: () => void) {
        this.response = response;
        this.flushed = flushed;
    }
    destroy() {
        if (this.destroyed) return;
        this.destroyed = true;
        this.emit("close");
    }
    pause() {}
}
async function fixture() {
    let accept!: (client: Client) => void;
    const timers = new Map<object, () => void>();
    const server = Object.assign(new EventEmitter(), {
        listen(_port: number, _host: string, ready: () => void) { ready(); },
        address() { return { port: 12345 }; },
        close(done: () => void) { done(); },
    });
    const source = readFileSync(new URL("./tunnel-gate.ts", import.meta.url), "utf8")
        .replace(/^import .*from "node:net";\n/, "")
        .replace("export async function", "async function");
    const context = {
        Buffer,
        createServer(handler: typeof accept) { accept = handler; return server; },
        connect() { return new Client(); },
        setTimeout(callback: () => void) { const id = {}; timers.set(id, callback); return id; },
        clearTimeout(id: object) { timers.delete(id); },
    };
    const start = runInNewContext(new Bun.Transpiler({ loader: "ts" }).transformSync(source) + "\nstartTunnelGate;", context);
    const gate = await start({ secret: "a".repeat(43), originPort: 12346, port: 0 });
    return {
        timers, gate,
        request(request: string) {
            const client = new Client();
            accept(client);
            client.emit("data", Buffer.from(request));
            return client;
        },
    };
}
const path = "/" + "a".repeat(43);
const upgrade = `GET ${path} HTTP/1.1\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n`;
for (const [request, status] of [
    ["GET /wrong HTTP/1.1\r\n\r\n", "404 Not Found"],
    [`GET ${path} HTTP/1.1\r\ninvalid header\r\n\r\n`, "400 Bad Request"],
    [`GET ${path} HTTP/1.1\r\n\r\n`, "200 OK"],
    [upgrade, "503 Service Unavailable"],
]) {
    const f = await fixture();
    try {
        if (status.startsWith("503")) {
            for (let i = 0; i < 32; i++) f.request(upgrade);
        }
        const originTimers = f.timers.size;
        const rejected = Array.from({ length: 64 }, () => f.request(request));
        assert(rejected.every((client) => client.response.startsWith(`HTTP/1.1 ${status}\r\n`)));
        assert.equal(f.timers.size, originTimers + 64, `${status}: retain absolute deadlines until flush`);
        assert(rejected.every((client) => !client.destroyed), "allow response to flush before destroying");
        assert(f.request(request).destroyed, "admission cap remains enforced during flush");
        for (const client of rejected) client.flushed?.();
        assert(rejected.every((client) => client.destroyed), `${status}: destroy after flushing without peer FIN`);
        assert.equal(f.timers.size, originTimers, "close cleans up deadlines");
        const stalled = f.request(request);
        assert(stalled.response.startsWith(`HTTP/1.1 ${status}\r\n`), "slots available again");
        stalled.emit("data", Buffer.from("trickle"));
        for (const expire of [...f.timers.values()]) expire();
        assert(stalled.destroyed, `${status}: stalled flush cannot outlive absolute deadline`);
    } finally {
        await f.gate.close();
    }
}
console.log("PASS: terminal 200/400/404/503 responses retain deadlines and release slots after flush");
