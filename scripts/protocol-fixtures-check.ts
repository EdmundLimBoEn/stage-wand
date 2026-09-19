import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const swift = readFileSync(join(root, "Shared/Protocol.swift"), "utf8");
const fixtures = JSON.parse(readFileSync(join(root, "Shared/protocol-fixtures.json"), "utf8"));
const schema = JSON.parse(readFileSync(join(root, "Shared/protocol.schema.json"), "utf8"));

function fail(message: string): never {
    console.error(`FAIL: ${message}`);
    process.exit(1);
}

const encodedTypes = [...swift.matchAll(/encode\("([a-zA-Z]+)", forKey: \.t\)/g)].map((match) => match[1]);
const expectedTypes = ["auth", "move", "scroll", "click", "key", "chord", "status", "bye"];
for (const type of expectedTypes) {
    if (!encodedTypes.includes(type)) fail(`Protocol.swift no longer encodes t=${type}`);
}
for (const type of encodedTypes) {
    if (!expectedTypes.includes(type)) fail(`Protocol.swift encodes new t=${type}; add fixtures and schema`);
}

const commandTypes = new Set(fixtures.commands.map((frame: { t: string }) => frame.t));
const replyTypes = new Set(fixtures.replies.map((frame: { t: string }) => frame.t));
for (const type of ["auth", "move", "click", "scroll", "key", "chord"]) {
    if (!commandTypes.has(type)) fail(`fixtures missing command t=${type}`);
}
for (const type of ["status", "bye"]) {
    if (!replyTypes.has(type)) fail(`fixtures missing reply t=${type}`);
}

const commandVariants = schema.$defs.command.oneOf.map((variant: { properties: { t: { const: string } } }) => variant.properties.t.const);
const replyVariants = schema.$defs.reply.oneOf.map((variant: { properties: { t: { const: string } } }) => variant.properties.t.const);
for (const type of commandTypes) {
    if (!commandVariants.includes(type)) fail(`schema missing command t=${type}`);
}
for (const type of replyTypes) {
    if (!replyVariants.includes(type)) fail(`schema missing reply t=${type}`);
}

if (!fixtures.invalidCommands.some((frame: string) => frame.includes("1e999"))) {
    fail("fixtures must keep the non-finite move case from protocol-check.swift");
}

const bluetoothSwift = readFileSync(join(root, "Shared/Bluetooth.swift"), "utf8");
const goUuids = readFileSync(join(root, "host/internal/ble/uuids.go"), "utf8");
const kotlinUuids = readFileSync(
    join(root, "android/protocol/src/main/kotlin/systems/edmundlim/stagewand/protocol/Bluetooth.kt"),
    "utf8",
);
for (const uuid of [
    "5A3E0001-8B6C-4B1E-9F8D-2C7A1D4E6F01",
    "5A3E0002-8B6C-4B1E-9F8D-2C7A1D4E6F01",
    "5A3E0003-8B6C-4B1E-9F8D-2C7A1D4E6F01",
]) {
    for (const [name, text] of [
        ["Shared/Bluetooth.swift", bluetoothSwift],
        ["host/internal/ble/uuids.go", goUuids],
        ["Bluetooth.kt", kotlinUuids],
    ] as const) {
        if (!text.toUpperCase().includes(uuid)) fail(`${name} missing ${uuid}`);
    }
}

console.log("PASS: Protocol.swift t values, schema variants, golden fixtures, and BLE UUIDs match");
