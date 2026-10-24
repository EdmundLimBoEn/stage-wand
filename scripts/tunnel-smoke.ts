// Opt-in network check: never run against a real user's tunnel configuration.
import { strict as assert } from "node:assert";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { homedir, tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));
const cloudflared = Bun.which("cloudflared") ?? Bun.which(join(homedir(), ".local/bin/cloudflared"));
if (!cloudflared) throw new Error("Install cloudflared or add it to PATH");
const scratch = await mkdtemp(join(tmpdir(), "stagewand-tunnel-smoke-"));
const env = { ...process.env, HOME: scratch, PATH: `${dirname(cloudflared)}:${process.env.PATH ?? ""}` };
const children: ReturnType<typeof Bun.spawn>[] = [];
let stopping = false;
function cancel() {
    stopping = true;
    for (const child of children) if (child.exitCode === null) child.kill("SIGTERM");
}
process.once("SIGINT", cancel);
process.once("SIGTERM", cancel);
const delay = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));
async function until(check: () => boolean | Promise<boolean>, label: string, ms = 120_000) {
    const deadline = Date.now() + ms;
    while (!await check()) {
        if (stopping) throw new Error("Interrupted");
        if (Date.now() > deadline) throw new Error(`Timed out: ${label}`);
        await delay(100);
    }
}
function spawn(cmd: string[], options: Parameters<typeof Bun.spawn>[1] = {}) {
    const child = Bun.spawn(cmd, { cwd: root, env, stdout: "pipe", stderr: "ignore", ...options });
    children.push(child);
    return child;
}
async function curl(url: string) {
    // Feed the capability via stdin, keeping it out of diagnostics and argv.
    const child = spawn(["curl", "--silent", "--max-time", "10", "--output", "/dev/null", "--write-out", "%{http_code}", "--config", "-"], {
        stdin: new Blob([`url = "${url}"\n`]),
    });
    const status = await new Response(child.stdout).text();
    return await child.exited === 0 ? status : "000";
}

try {
    const binary = join(scratch, "stagewand-host");
    // Build with the existing Go cache; only runtime processes need isolated HOME.
    const build = spawn(["go", "build", "-o", binary, "./cmd/stagewand-host"], {
        cwd: join(root, "host"), env: process.env, stdout: "inherit", stderr: "inherit",
    });
    assert.equal(await build.exited, 0, "Go build failed");
    if (stopping) throw new Error("Interrupted");
    const host = spawn([binary, "--serve"]);
    let port: string | undefined;
    const moves: { dx: number; dy: number }[] = [];
    const hostOutput = (async () => {
        let pending = "";
        const decoder = new TextDecoder();
        for await (const chunk of host.stdout as ReadableStream<Uint8Array>) {
            pending += decoder.decode(chunk, { stream: true });
            const lines = pending.split("\n");
            pending = lines.pop()!;
            for (const line of lines) {
                if (line.startsWith("PORT ")) port = line.slice(5).trim();
                if (line.startsWith("COMMAND ")) {
                    const command = JSON.parse(line.slice(8));
                    if (command.t === "move") moves.push(command);
                }
            }
        }
    })();
    await until(() => {
        if (host.exitCode !== null) throw new Error("Host exited before startup");
        return Boolean(port);
    }, "host startup");
    console.log("PASS: no-op host started with isolated HOME");
    const tunnel = spawn([process.execPath, "scripts/tunnel.ts"], {
        env: { ...env, STAGEWAND_PORT: port!, STAGEWAND_TUNNEL_PORT: "0" },
        stdout: "inherit", stderr: "inherit",
    });
    let url = "";
    await until(async () => {
        if (tunnel.exitCode !== null) throw new Error("Tunnel runner exited before publishing URL");
        try { url = (await readFile(join(scratch, ".config/stage-wand/tunnel-url"), "utf8")).trim(); } catch {}
        return Boolean(url);
    }, "Quick Tunnel URL");
    const endpoint = new URL(url);
    assert.equal(endpoint.protocol, "wss:");
    assert(endpoint.hostname.endsWith(".trycloudflare.com"));
    const httpsURL = url.replace(/^wss:/, "https:");
    let publicStatus = "000";
    try {
        await until(async () => {
            if (tunnel.exitCode !== null) throw new Error("Tunnel runner exited during public readiness check");
            publicStatus = await curl(httpsURL);
            if (publicStatus !== "200") await delay(1000);
            return publicStatus === "200";
        }, "public secret-path GET (DNS/tunnel startup)");
    } catch (error) {
        console.error(`Public secret-path GET: HTTP ${publicStatus}; wrong-path GET: HTTP ${await curl(`https://${endpoint.host}/wrong-path`)}`);
        throw error;
    }
    console.log("PASS: public secret-path GET → 200 (path redacted)");
    assert.equal(await curl(`https://${endpoint.host}/wrong-path`), "404", "Public wrong path must return 404");
    console.log("PASS: public wrong-path GET → 404");
    const smoke = spawn([process.execPath, "scripts/ws-smoke.ts"], {
        env: { ...env, STAGEWAND_SMOKE_URL: url }, stdout: "inherit", stderr: "inherit",
    });
    await until(() => smoke.exitCode !== null, "public WebSocket smoke", 60_000);
    assert.equal(await smoke.exited, 0, "Public WebSocket smoke failed");
    await until(() => moves.length >= 200, "host command receipt", 5000);
    assert.equal(moves.length, 200, "Invalid moves must be dropped");
    moves.forEach((move, i) => assert.deepEqual([move.dx, move.dy], [i, i === 0 ? 0 : -i]));
    console.log("PASS: host received all 200 public-tunnel moves in order; invalid deltas dropped");
    host.kill();
    await host.exited;
    await hostOutput;
    console.log("PASS: public tunnel smoke complete");
} catch (error) {
    // Never dump exception objects that could contain the private URL.
    console.error("FAIL:", error instanceof Error ? error.message.replace(/(?:wss?|https?):\/\/\S+/g, "[redacted URL]") : "Tunnel smoke failed");
    process.exitCode = 1;
} finally {
    cancel();
    // Give tunnel.ts time to reap cloudflared and remove its URL file.
    const force = setTimeout(() => {
        for (const child of children) if (child.exitCode === null) child.kill("SIGKILL");
    }, 5000);
    await Promise.all(children.map((child) => child.exited));
    clearTimeout(force);
    await rm(scratch, { recursive: true, force: true });
    process.removeListener("SIGINT", cancel);
    process.removeListener("SIGTERM", cancel);
}
