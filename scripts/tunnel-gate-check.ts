import { strict as assert } from "node:assert";
import { randomBytes } from "node:crypto";
import { connect, type Socket } from "node:net";
import { startTunnelGate } from "./tunnel-gate";

let upgrades = 0;
let closes = 0;
let pong = "";
let clientPing = "";
let originPath = "";
let originHeader = "";
const origin = Bun.serve({
    hostname: "127.0.0.1", port: 0,
    fetch(request, server) {
        upgrades++;
        originPath = new URL(request.url).pathname;
        originHeader = request.headers.get("x-test-header") ?? "";
        if (server.upgrade(request)) return;
        return new Response("Upgrade required", { status: 400 });
    },
    websocket: {
        message(ws, message) {
            const auth = JSON.parse(String(message));
            assert.deepEqual(auth, { t: "auth", code: "1234" });
            ws.send(JSON.stringify({ t: "status", ok: true }));
            ws.ping("server-ping");
        },
        pong(_ws, payload) { pong = payload.toString(); },
        ping(_ws, payload) { clientPing = payload.toString(); },
        close() { closes++; },
    },
});
const secret = randomBytes(32).toString("base64url");
const gate = await startTunnelGate({ secret, originPort: origin.port!, port: 0 });
const clients: Socket[] = [];
const delay = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));
async function until(check: () => boolean, label: string) {
    const deadline = Date.now() + 4000;
    while (!check()) {
        if (Date.now() > deadline) throw new Error(`Timed out: ${label}`);
        await delay(10);
    }
}
function frame(opcode: number, payload: string | Buffer) {
    const data = Buffer.from(payload);
    assert(data.length < 126);
    return Buffer.concat([Buffer.from([0x80 | opcode, 0x80 | data.length, 0, 0, 0, 0]), data]);
}
function raw(path: string) {
    const socket = connect(gate.port, "127.0.0.1");
    clients.push(socket);
    socket.on("error", () => {});
    socket.on("connect", () => socket.write(`GET ${path} HTTP/1.1\r\nHost: example.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: ${randomBytes(16).toString("base64")}\r\nSec-WebSocket-Version: 13\r\nX-Test-Header: preserved\r\n\r\n`));
    return socket;
}
try {
    assert.equal((await fetch(`http://127.0.0.1:${gate.port}/wrong`)).status, 404);
    assert.equal((await fetch(`http://127.0.0.1:${gate.port}/${secret}`)).status, 200);
    for (const path of ["/", "/wrong", `/${secret}?query=1`, `/${secret}/`]) {
        const rejected = raw(path);
        let reply = "";
        let closed = false;
        rejected.on("close", () => { closed = true; });
        rejected.on("data", (chunk) => { reply += chunk.toString(); });
        await until(() => closed, "rejected connection close");
        assert(reply.startsWith("HTTP/1.1 404"), JSON.stringify(reply));
    }
    assert.equal(upgrades, 0);
    console.log("PASS: unknown paths blocked before origin; private health GET works");

    const socket = raw(`/${secret}`);
    let pending = Buffer.alloc(0);
    let handshake = false;
    const received: [number, string][] = [];
    socket.on("data", (chunk) => {
        pending = Buffer.concat([pending, chunk]);
        if (!handshake) {
            const end = pending.indexOf("\r\n\r\n");
            if (end < 0) return;
            assert(pending.subarray(0, end).toString().startsWith("HTTP/1.1 101"));
            pending = pending.subarray(end + 4);
            handshake = true;
            socket.write(frame(1, JSON.stringify({ t: "auth", code: "1234" })));
            socket.write(frame(9, "client-ping"));
        }
        while (pending.length >= 2) {
            const size = pending[1] & 127;
            assert(size < 126);
            if (pending.length < size + 2) return;
            const opcode = pending[0] & 15;
            const payload = pending.subarray(2, size + 2);
            received.push([opcode, payload.toString()]);
            if (opcode === 9) socket.write(frame(10, payload));
            if (opcode === 8) socket.end();
            pending = pending.subarray(size + 2);
        }
    });
    await until(() => pong === "server-ping" && clientPing === "client-ping" && received.some(([opcode, payload]) => opcode === 10 && payload === "client-ping"), "bidirectional ping/pong");
    assert.deepEqual(JSON.parse(received.find(([opcode]) => opcode === 1)![1]), { t: "status", ok: true });
    assert(received.findIndex(([opcode]) => opcode === 1) < received.findIndex(([opcode]) => opcode === 9));
    assert.equal(originPath, "/");
    assert.equal(originHeader, "preserved");
    socket.write(frame(8, Buffer.from([3, 232])));
    await until(() => socket.destroyed && closes === 1, "clean WebSocket close");
    console.log("PASS: auth/reply, rewritten path, headers, ping/pong order, clean close");

    const live = Array.from({ length: 32 }, () => raw(`/${secret}`));
    await until(() => upgrades === 33, "32 simultaneous upgrades");
    const excess = raw(`/${secret}`);
    let excessReply = "";
    excess.on("data", (chunk) => { excessReply += chunk.toString(); });
    await until(() => excessReply.startsWith("HTTP/1.1 503"), "connection cap rejection");
    assert.equal(upgrades, 33);
    console.log("PASS: 32-upgrade cap rejects excess before origin");
    await gate.close();
    await until(() => live.every((socket) => socket.destroyed) && closes === 33, "gate shutdown cleanup");
    console.log("PASS: gate shutdown closes active origin and client sockets");
} finally {
    for (const socket of clients) socket.destroy();
    await gate.close();
    origin.stop(true);
}
