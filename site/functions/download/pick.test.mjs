import assert from "node:assert/strict";
import { test } from "node:test";
const { pickAsset, PLATFORMS, NAMES, fallbackAsset, tagFromLocation } = await import("./[platform].js");
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
const newerDeb = [{ tag_name: "linux", published_at: "2026-12-01T00:00:00Z", assets: [{ name: "stagewand-host_amd64.deb" }] }, ...L];
assert.equal(n("linux", newerDeb), "StageWand-0.2.0-linux-x86_64.AppImage");
assert.equal(n("appimage"), "StageWand-0.2.0-linux-x86_64.AppImage");
assert.equal(n("linux", L.slice(1)), "stagewand-host_amd64.deb");

// Fallback (API down): tag from the /releases/latest redirect + documented names.
const base = "https://github.com/EdmundLimBoEn/stage-wand/releases";
assert.equal(tagFromLocation(`${base}/tag/v0.1.0`), "v0.1.0");
assert.equal(tagFromLocation(`${base}/tag/mac-v0.1.0`), null);
assert.equal(tagFromLocation(`${base}`), null);
assert.equal(tagFromLocation(null), null);
const want = {
  mac: "StageWand-0.1.0.dmg", windows: "StageWand-0.1.0-windows-x64.zip",
  linux: "StageWand-0.1.0-linux-x86_64.AppImage", appimage: "StageWand-0.1.0-linux-x86_64.AppImage",
  deb: "StageWand-0.1.0-linux-amd64.deb", arch: "StageWand-0.1.0-linux-x86_64.pkg.tar.zst",
  android: "StageWand-0.1.0-android.apk",
};
assert.deepEqual(Object.keys(NAMES).sort(), Object.keys(PLATFORMS).sort());
for (const [p, f] of Object.entries(want)) {
  const a = fallbackAsset("v0.1.0", p);
  assert.equal(a.name, f);
  assert.equal(a.browser_download_url, `${base}/download/v0.1.0/${f}`);
  // The API path picks a file matching the same platform patterns.
  assert.ok(PLATFORMS[p].some((re) => re.test(f)), p);
}
assert.equal(fallbackAsset("linux", "deb"), null);
assert.equal(fallbackAsset("v0.1.0", "toString"), null);
assert.equal(fallbackAsset(null, "mac"), null);
console.log("pickAsset ok");

const { onRequest } = await import("./[platform].js");
for (const scenario of [
  { name: "missing guessed DMG uses stale release", guessed: 404, stale: true, older: 200, status: 200 },
  { name: "successful guessed DMG skips stale cache", guessed: 200, stale: true, older: 200, status: 200 },
  { name: "missing guessed DMG without stale match returns 502", guessed: 404, stale: false, older: 200, status: 502 },
  { name: "missing guessed and stale DMGs return 502", guessed: 404, stale: true, older: 404, status: 502 },
  { name: "rejected guessed DMG uses stale release", guessed: "throw", stale: true, older: 200, status: 200 },
  { name: "rejected guessed DMG without stale match returns 502", guessed: "throw", stale: false, older: 200, status: 502 },
  { name: "rejected guessed and stale DMGs return 502", guessed: "throw", stale: true, older: "throw", status: 502 },
  { name: "missing guessed and rejected stale DMGs return 502", guessed: 404, stale: true, older: "throw", status: 502 },
]) {
  test(scenario.name, async (t) => {
    const guessed = fallbackAsset("v0.2.0", "mac");
    const older = fallbackAsset("v0.1.0", "mac");
    const downloads = [];
    let staleReads = 0;
    const originalCaches = Object.getOwnPropertyDescriptor(globalThis, "caches");
    Object.defineProperty(globalThis, "caches", { configurable: true, value: { default: {
      async match(request) {
        if (!request.url.includes("/releases-stale/")) return undefined;
        staleReads++;
        return Response.json([{ assets: scenario.stale ? [older] : [] }]);
      },
    } } });
    t.after(() => {
      if (originalCaches) Object.defineProperty(globalThis, "caches", originalCaches);
      else delete globalThis.caches;
    });
    t.mock.method(globalThis, "fetch", async (url) => {
      if (url.startsWith("https://api.github.com/")) return new Response(null, { status: 403 });
      if (url === `${base}/latest`) return new Response(null, {
        status: 302, headers: { Location: `${base}/tag/v0.2.0` },
      });
      downloads.push(url);
      if (url === guessed.browser_download_url) {
        if (scenario.guessed === "throw") throw new TypeError("fetch failed");
        return new Response("latest DMG", { status: scenario.guessed });
      }
      assert.equal(url, older.browser_download_url);
      if (scenario.older === "throw") throw new TypeError("fetch failed");
      return new Response("older DMG", { status: scenario.older, headers: { "Content-Length": "9" } });
    });
    const response = await onRequest({
      params: { platform: "mac" }, request: new Request("https://stagewand.example/download/mac"),
    });
    assert.equal(response.status, scenario.status);
    assert.equal(staleReads, scenario.guessed === 200 ? 0 : 1);
    assert.deepEqual(downloads, [guessed.browser_download_url,
      ...(scenario.guessed !== 200 && scenario.stale ? [older.browser_download_url] : [])]);
    if (response.ok) {
      const selected = scenario.guessed === 200 ? guessed : older;
      assert.equal(response.headers.get("Content-Disposition"), `attachment; filename="${selected.name}"`);
      assert.equal(await response.text(), scenario.guessed === 200 ? "latest DMG" : "older DMG");
      if (selected === older) assert.equal(response.headers.get("Content-Length"), "9");
    }
  });
}
