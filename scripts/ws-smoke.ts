import { strict as assert } from "node:assert";
import { connect as tcpConnect } from "node:net";
import { randomBytes } from "node:crypto";

const args = process.argv.slice(2);
const spawnHost = args.includes("--spawn-host");
const spawnServer = args.includes("--spawn") || spawnHost;
const explicitURL = args.find((arg) => arg.startsWith("ws://"));
const sockets: WebSocket[] = [];
let child: ReturnType<typeof Bun.spawn> | undefined;
let output = "";
const observedMoves: { dx: number; dy: number }[] = [];
const delay = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));
async function until(predicate: () => boolean, label: string, ms = 5000) {
    const deadline = performance.now() + ms;
    while (!predicate()) {
        if (performance.now() > deadline) throw new Error(`Timed out: ${label}`);
        await delay(10);
    }
}
function connect(url: string) {
    const ws = new WebSocket(url);
    sockets.push(ws);
    const messages: any[] = [];
    let closedAt = 0;
    const createdAt = performance.now();
    ws.addEventListener("message", (event) => messages.push(JSON.parse(String(event.data))));
    ws.addEventListener("close", () => { closedAt = performance.now(); });
    const opened = new Promise<void>((resolve, reject) => {
        ws.addEventListener("open", () => resolve(), { once: true });
        ws.addEventListener("error", () => reject(new Error(`Cannot connect: ${url}`)), { once: true });
    });
    return { ws, messages, opened, get closedAt() { return closedAt; }, createdAt };
}
async function authenticated(url: string) {
    const peer = connect(url);
    await peer.opened;
    peer.ws.send(JSON.stringify({ t: "auth", code: "0000" }));
    await until(() => peer.messages.some((m) => m.t === "status"), "auth status");
    return peer;
}

async function withoutPong(url: string) {
    const endpoint = new URL(url);
    const socket = tcpConnect(Number(endpoint.port), endpoint.hostname);
    let pending = Buffer.alloc(0);
    let upgraded = false;
    let authedAt = 0;
    let closedAt = 0;
    let pings = 0;
    socket.on("error", () => {});
    socket.on("close", () => { closedAt = performance.now(); });
    socket.on("connect", () => socket.write(
        `GET / HTTP/1.1\r\nHost: ${endpoint.host}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: ${randomBytes(16).toString("base64")}\r\nSec-WebSocket-Version: 13\r\n\r\n`,
    ));
    socket.on("data", (chunk) => {
        pending = Buffer.concat([pending, chunk]);
        if (!upgraded) {
            const end = pending.indexOf("\r\n\r\n");
            if (end < 0) return;
            assert(pending.subarray(0, end).toString().includes("101"));
            pending = pending.subarray(end + 4);
            upgraded = true;
            const auth = Buffer.from(JSON.stringify({ t: "auth", code: "0000" }));
            socket.write(Buffer.concat([Buffer.from([0x81, 0x80 | auth.length, 0, 0, 0, 0]), auth]));
        }
        while (pending.length >= 2) {
            const size = pending[1] & 0x7f;
            if (size >= 126 || pending.length < size + 2) return;
            const opcode = pending[0] & 0x0f;
            if (opcode === 1 && JSON.parse(pending.subarray(2, size + 2).toString()).t === "status") authedAt = performance.now();
            if (opcode === 9) pings++;
            pending = pending.subarray(size + 2);
        }
    });
    try {
        await until(() => authedAt > 0, "raw client auth");
        await until(() => closedAt > 0, "missing-pong close", 9000);
        assert(pings >= 1, "Server must send ping");
        assert(closedAt - authedAt >= 5500 && closedAt - authedAt < 8500);
        console.log(`PASS: no pong → closed in ${((closedAt - authedAt) / 1000).toFixed(2)} s`);
    } finally { socket.destroy(); }
}

async function spawnGoHost(hostDir: string) {
    const binary = "/tmp/stagewand-host-smoke";
    const build = Bun.spawn(["go", "build", "-o", binary, "./cmd/stagewand-host"], {
        cwd: hostDir,
        stdout: "inherit",
        stderr: "inherit",
    });
    const status = await build.exited;
    if (status !== 0) throw new Error(`go build failed with ${status}`);
    return Bun.spawn([binary, "--serve"], { stdout: "pipe", stderr: "inherit" });
}

try {
    let url = explicitURL ?? "ws://127.0.0.1:8787";
    if (spawnServer) {
        const repoRoot = new URL("..", import.meta.url).pathname;
        child = spawnHost
            ? await spawnGoHost(`${repoRoot}host`)
            : Bun.spawn(["swift", "run", "--package-path", "mac", "StageWandMac", "--serve"], {
                cwd: repoRoot,
                stdout: "pipe",
                stderr: "inherit",
            });
        void (async () => {
            const reader = child!.stdout as ReadableStream<Uint8Array>;
            const decoder = new TextDecoder();
            let pending = "";
            for await (const chunk of reader) {
                const text = decoder.decode(chunk, { stream: true });
                output += text;
                pending += text;
                const lines = pending.split("\n");
                pending = lines.pop()!;
                for (const line of lines) {
                    if (line.startsWith("COMMAND ")) {
                        const command = JSON.parse(line.slice(8));
                        if (command.t === "move") observedMoves.push(command);
                    }
                }
            }
        })();
        await until(() => /PORT (\d+)/.test(output), "server startup", 120_000);
        url = `ws://127.0.0.1:${output.match(/PORT (\d+)/)![1]}`;
    }
    console.log(`Testing ${url}`);
    const idle = connect(url);
    await idle.opened;
    await until(() => idle.closedAt > 0, "no-auth close");
    const elapsed = idle.closedAt - idle.createdAt;
    assert(elapsed >= 1500 && elapsed < 3500, `Auth deadline: ${elapsed} ms`);
    console.log(`PASS: no auth → closed in ${(elapsed / 1000).toFixed(2)} s`);

    const bad = connect(url);
    await bad.opened;
    bad.ws.send(JSON.stringify({ t: "auth", code: "wrong" }));
    await until(() => bad.closedAt > 0, "bad-auth close");
    assert.deepEqual(bad.messages[0], { t: "bye", reason: "badauth" });
    console.log("PASS: bad code → bye badauth + close");

    const early = connect(url);
    await early.opened;
    early.ws.send(JSON.stringify({ t: "key", k: "right" }));
    await until(() => early.closedAt > 0, "non-auth first frame close");
    console.log("PASS: non-auth first frame → closed");

    const first = await authenticated(url);
    console.log("PASS: good code → status");
    const second = await authenticated(url);
    await until(() => first.closedAt > 0, "displaced close");
    assert(first.messages.some((m) => m.t === "bye" && m.reason === "displaced"));
    console.log("PASS: second good code → first gets bye displaced + close");

    second.ws.send('{"t":"move","dx":1e999,"dy":0}');
    second.ws.send(JSON.stringify({ t: "move", dx: 401, dy: 0 }));
    for (let i = 0; i < 200; i++) second.ws.send(JSON.stringify({ t: "move", dx: i, dy: -i }));
    if (spawnServer) {
        await until(() => observedMoves.length >= 200, "200 received moves");
        assert.equal(observedMoves.length, 200);
        observedMoves.forEach((move, i) => assert.deepEqual([move.dx, move.dy], [i, i === 0 ? 0 : -i]));
        assert(output.includes("MOVES 200"));
        console.log("PASS: 200 moves received in order; invalid deltas dropped; server count MOVES 200");
    } else {
        await delay(300);
        console.log("SENT: 200 ordered moves; verify MOVES 200 in server output (use --spawn for automatic receipt checks)");
    }
    await delay(7000);
    assert.equal(second.ws.readyState, WebSocket.OPEN);
    console.log("PASS: pong replies keep authenticated peer alive beyond 6 s");
    second.ws.close();
    await withoutPong(url);
    console.log("PASS: WebSocket smoke checks complete");
} catch (error) {
    console.error("FAIL:", error);
    process.exitCode = 1;
} finally {
    for (const ws of sockets) ws.close();
    child?.kill();
    if (child) await child.exited;
}
