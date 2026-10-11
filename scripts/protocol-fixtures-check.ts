import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const read = (path: string) => readFileSync(join(root, path), "utf8");
const swift = read("Shared/Protocol.swift");
const fixtures = JSON.parse(read("Shared/protocol-fixtures.json"));
const schema = JSON.parse(read("Shared/protocol.schema.json"));

function fail(message: string): never {
    console.error(`FAIL: ${message}`);
    process.exit(1);
}

// This checks only the JSON Schema features present in the checked-in wire schema.
function valid(value: unknown, rule: Record<string, any>): boolean {
    if (rule.$ref) {
        const target = rule.$ref.split("/").slice(1).reduce((node: any, key: string) => node[key], schema);
        return valid(value, target);
    }
    if (rule.oneOf) return rule.oneOf.filter((candidate: Record<string, any>) => valid(value, candidate)).length === 1;
    if (rule.const !== undefined && value !== rule.const) return false;
    if (rule.enum && !rule.enum.includes(value)) return false;
    if (rule.type === "string" && (typeof value !== "string" || !value.isWellFormed())) return false;
    if (rule.type === "number") {
        if (typeof value !== "number" || !Number.isFinite(value)) return false;
        if (rule.minimum !== undefined && value < rule.minimum) return false;
        if (rule.maximum !== undefined && value > rule.maximum) return false;
    }
    if (rule.type === "object") {
        if (value === null || typeof value !== "object" || Array.isArray(value)) return false;
        const object = value as Record<string, unknown>;
        if (rule.required?.some((key: string) => !Object.hasOwn(object, key))) return false;
        for (const key of Object.keys(object)) {
            if (!rule.properties?.[key]) {
                if (rule.additionalProperties === false) return false;
            } else if (!valid(object[key], rule.properties[key])) return false;
        }
    }
    return true;
}

function uniqueKeys(text: string): boolean {
    const keys = new Set<string>();
    let depth = 0;
    let expectingKey = true;
    for (let index = 0; index < text.length; index++) {
        const character = text[index];
        if (character === '"') {
            const start = index++;
            while (index < text.length) {
                if (text[index] === "\\") { index += 2; continue; }
                if (text[index] === '"') break;
                index++;
            }
            if (index >= text.length) return false;
            if (depth === 1 && expectingKey) {
                const key = JSON.parse(text.slice(start, index + 1));
                if (keys.has(key)) return false;
                keys.add(key);
            }
        } else if (character === "{" || character === "[") depth++;
        else if (character === "}" || character === "]") depth--;
        else if (depth === 1 && character === ",") expectingKey = true;
        else if (depth === 1 && character === ":") expectingKey = false;
    }
    return true;
}

for (const [frames, definition] of [[fixtures.commands, schema.$defs.command], [fixtures.replies, schema.$defs.reply]]) {
    for (const frame of frames) {
        if (!valid(frame, definition)) fail(`golden fixture violates schema: ${JSON.stringify(frame)}`);
    }
}
for (const [frames, definition] of [[fixtures.invalidCommands, schema.$defs.command], [fixtures.invalidReplies, schema.$defs.reply]]) {
    if (!frames?.length) fail("missing malformed-frame coverage");
    for (const frame of frames) {
        let parsed: unknown;
        try { parsed = JSON.parse(frame); } catch { continue; }
        if (uniqueKeys(frame) && valid(parsed, definition)) fail(`invalid fixture satisfies schema: ${frame}`);
    }
}

for (const text of fixtures.validCommandTexts) {
    if (!uniqueKeys(text) || !valid(JSON.parse(text), schema.$defs.command)) fail(`valid raw fixture rejected: ${text}`);
}

const encodedTypes = [...swift.matchAll(/encode\("([a-zA-Z]+)", forKey: \.t\)/g)].map((match) => match[1]);
const expectedTypes = ["auth", "move", "scroll", "click", "key", "chord", "status", "bye"];
if (new Set(encodedTypes).size !== expectedTypes.length || expectedTypes.some((type) => !encodedTypes.includes(type))) {
    fail("Swift command/reply types changed; update schema and golden fixtures");
}
for (const [frames, definition] of [[fixtures.commands, schema.$defs.command], [fixtures.replies, schema.$defs.reply]]) {
    const fixtureTypes = new Set(frames.map((frame: { t: string }) => frame.t));
    const schemaTypes = new Set(definition.oneOf.map((variant: any) => variant.properties.t.const));
    if (fixtureTypes.size !== schemaTypes.size || [...fixtureTypes].some((type) => !schemaTypes.has(type))) {
        fail("schema types and golden fixture types differ");
    }
}
for (const [type, field, definition] of [["click", "b", "button"], ["key", "k", "key"], ["chord", "k", "chord"]]) {
    for (const value of schema.$defs[definition].enum) {
        if (!fixtures.commands.some((frame: any) => frame.t === type && frame[field] === value)) {
            fail(`missing fixture for ${type}.${value}`);
        }
    }
}

const bluetooth = fixtures.bluetooth;
if (JSON.stringify(bluetooth) !== JSON.stringify(schema["x-bluetooth"])) fail("BLE fixture/schema metadata differ");
const sources = [
    { path: "Shared/Bluetooth.swift", names: ["service", "command", "reply", "maxFrameBytes", "minimumCommandBytes"] },
    { path: "host/internal/ble/uuids.go", names: ["ServiceUUID", "CommandUUID", "ReplyUUID"] },
    { path: "host/internal/protocol/protocol.go", names: ["MaxFrameBytes", "MinimumCommandBytes"], offset: 3 },
    { path: "android/protocol/src/main/kotlin/systems/edmundlim/stagewand/protocol/Bluetooth.kt", names: ["SERVICE", "COMMAND", "REPLY", "MAX_FRAME_BYTES", "MIN_COMMAND_BYTES"] },
];
const keys = ["service", "command", "reply", "maxFrameBytes", "minimumCommandBytes"];
for (const { path, names, offset = 0 } of sources) {
    const source = read(path);
    for (let index = 0; index < names.length; index++) {
        const match = source.match(new RegExp(`\\b${names[index]}\\s*=\\s*(?:"([^"]+)"|(\\d+))`));
        if (!match || String(bluetooth[keys[index + offset]]) !== (match[1] ?? match[2])) {
            fail(`${path}: ${names[index]} differs from shared BLE contract`);
        }
    }
}
for (const frame of fixtures.commands) {
    if (Buffer.byteLength(JSON.stringify(frame), "utf8") > bluetooth.minimumCommandBytes) {
        fail(`canonical command exceeds minimum BLE payload: ${JSON.stringify(frame)}`);
    }
}
console.log(`PASS: ${fixtures.commands.length} commands, ${fixtures.replies.length} replies, ${fixtures.invalidCommands.length + fixtures.invalidReplies.length} malformed frames, schema and BLE constants`);
