import assert from "node:assert/strict";
const { pickAsset, PLATFORMS } = await import("./[platform].js");
const L = [
  { tag_name: "v0.2.0", published_at: "2026-11-01T00:00:00Z", assets: [
    { name: "StageWand-0.2.0.dmg" }, { name: "StageWand-0.2.0.dmg.sha256" },
    { name: "StageWand-0.2.0-windows-x64.zip" }, { name: "StageWand-0.2.0-linux-x86_64.AppImage" } ] },
  { tag_name: "mac-v0.1.0", published_at: "2026-10-17T00:00:00Z", assets: [{ name: "StageWand-0.1.0.dmg" }] },
  { tag_name: "linux", published_at: "2026-10-11T00:00:00Z", assets: [
    { name: "stagewand-host_0.1.0-3+git1_amd64.deb" }, { name: "stagewand-host_amd64.deb" },
    { name: "stagewand-host-x86_64.pkg.tar.zst" }, { name: "SHA256SUMS" } ] },
];
const n = (p, l = L) => pickAsset(l, PLATFORMS[p])?.name;
assert.equal(n("mac"), "StageWand-0.2.0.dmg");
assert.equal(n("windows"), "StageWand-0.2.0-windows-x64.zip");
assert.equal(n("linux"), "StageWand-0.2.0-linux-x86_64.AppImage");
assert.equal(n("deb"), "stagewand-host_amd64.deb");
assert.equal(n("arch"), "stagewand-host-x86_64.pkg.tar.zst");
assert.equal(n("android"), undefined);
assert.equal(n("mac", L.slice(1)), "StageWand-0.1.0.dmg");
assert.equal(n("windows", L.slice(1)), undefined);
console.log("pickAsset ok");
