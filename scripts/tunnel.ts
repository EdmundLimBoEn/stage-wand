import { randomBytes } from "node:crypto";
import { chmodSync, existsSync, mkdirSync, readFileSync, renameSync, unlinkSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import { startTunnelGate } from "./tunnel-gate";

function port(value: string | undefined, fallback: number, allowZero = false) {
    const result = value === undefined ? fallback : Number(value);
    if (!Number.isInteger(result) || result < (allowZero ? 0 : 1) || result > 65535) throw new Error("Invalid port");
    return result;
}

let gate: Awaited<ReturnType<typeof startTunnelGate>> | undefined;
let child: ReturnType<typeof Bun.spawn> | undefined;
let ownedURL: string | undefined;
let stopping = false;
let cleanup: Promise<void> | undefined;
const directory = join(homedir(), ".config", "stage-wand");
const urlFile = join(directory, "tunnel-url");
const pathFile = join(directory, "tunnel-path");
const tokenFile = join(directory, "tunnel-token");
const hostFile = join(directory, "named-host");
const temporaryFile = `${urlFile}.${process.pid}.tmp`;
function stop() {
    stopping = true;
    return cleanup ??= (async () => {
        child?.kill("SIGTERM");
        const forceKill = setTimeout(() => child?.kill("SIGKILL"), 3000);
        forceKill.unref();
        await gate?.close();
        if (child) await child.exited;
        clearTimeout(forceKill);
        try { if (ownedURL && readFileSync(urlFile, "utf8") === ownedURL) unlinkSync(urlFile); } catch {}
        try { unlinkSync(temporaryFile); } catch {}
    })();
}
process.once("SIGINT", () => { void stop(); });
process.once("SIGTERM", () => { void stop(); });

try {
    mkdirSync(directory, { recursive: true, mode: 0o700 });
    chmodSync(directory, 0o700);
    if (!existsSync(pathFile)) writeFileSync(pathFile, `${randomBytes(32).toString("base64url")}\n`, { mode: 0o600, flag: "wx" });
    chmodSync(pathFile, 0o600);
    const secret = readFileSync(pathFile, "utf8").trim();
    const named = existsSync(tokenFile);
    const hostname = named ? (process.env.STAGEWAND_TUNNEL_HOST ?? readFileSync(hostFile, "utf8")).trim() : undefined;
    if (hostname && !/^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/i.test(hostname)) throw new Error("Invalid hostname");
    if (named && !hostname) throw new Error("Missing hostname");
    if (named) chmodSync(tokenFile, 0o600);
    function publish(host: string) {
        ownedURL = `wss://${host}/${secret}\n`;
        writeFileSync(temporaryFile, ownedURL, { mode: 0o600, flag: "wx" });
        renameSync(temporaryFile, urlFile);
        console.log(`Tunnel hostname: ${host}`);
        console.log("Private connection URL saved to ~/.config/stage-wand/tunnel-url");
    }
    gate = await startTunnelGate({
        secret,
        port: port(process.env.STAGEWAND_TUNNEL_PORT, 18788, true),
        originPort: port(process.env.STAGEWAND_PORT, 8787),
    });
    if (!stopping) {
        const arguments_ = ["cloudflared", "tunnel", "--no-autoupdate", "--protocol", "http2"];
        if (named) arguments_.push("run", "--token-file", tokenFile);
        else arguments_.push("--url", `http://127.0.0.1:${gate.port}`);
        child = Bun.spawn(arguments_, { stdin: "ignore", stdout: "ignore", stderr: "pipe" });
        if (hostname) publish(hostname);
        const decoder = new TextDecoder();
        let pending = "";
        let diagnosticPending = "";
        const diagnostics = new Set<string>();
        function diagnostic(line: string) {
            let message: string | undefined;
            if (line.includes("Registered tunnel connection")) message = "Tunnel connection ready.";
            else if (/\b(?:ERR|WRN|WARN)\b/.test(line)) {
                const category = /timeout|deadline exceeded/i.test(line) ? "connection timeout" :
                    /refused|unreachable|no route|network/i.test(line) ? "network connectivity" :
                    /unauthorized|forbidden|invalid token|authentication/i.test(line) ? "authentication" :
                    /certificate|tls|x509/i.test(line) ? "TLS configuration" :
                    /dns|lookup|resolve/i.test(line) ? "DNS resolution" :
                    /origin|localhost|127\.0\.0\.1/i.test(line) ? "local origin connection" : "connection or configuration";
                message = `Tunnel diagnostic: ${category}.`;
            }
            if (message && !diagnostics.has(message)) {
                diagnostics.add(message);
                console.error(message);
            }
        }
        for await (const chunk of child.stderr as ReadableStream<Uint8Array>) {
            if (stopping) continue;
            const decoded = decoder.decode(chunk, { stream: true });
            pending = (pending + decoded).slice(-16_384);
            diagnosticPending = (diagnosticPending + decoded).slice(-16_384);
            const lines = diagnosticPending.split("\n");
            diagnosticPending = lines.pop()!;
            for (const line of lines) diagnostic(line);
            const match = pending.match(/https:\/\/([a-z0-9-]+\.trycloudflare\.com)\b/i);
            if (match && !ownedURL) publish(match[1]);
        }
        const status = await child.exited;
        if (!stopping) {
            console.error(`Tunnel process exited (${status}).`);
            process.exitCode = status || 1;
        }
    }
} catch {
    console.error("Could not start or maintain tunnel. Check cloudflared, local ports, and ~/.config/stage-wand configuration.");
    process.exitCode = 1;
} finally {
    await stop();
    await gate?.close();
}
