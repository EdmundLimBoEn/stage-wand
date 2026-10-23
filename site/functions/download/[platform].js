// Stable download URLs: /download/mac, /download/linux (.deb), /download/arch,
// /download/windows. Resolves the newest matching GitHub release and proxies
// the asset so visitors never land on GitHub.
const REPO = "EdmundLimBoEn/stage-wand";
// Asset matching is by platform keyword + extension, across every release of
// any tag scheme (mac-v*, linux, v*), newest published first. Each platform
// lists patterns in preference order; the first pattern matched in the newest release wins.
export const PLATFORMS = {
  mac:     [/\.dmg$/i],
  windows: [/win(dows)?.*\.(msi|exe|zip)$/i, /\.(msi|exe)$/i],
  appimage: [/\.appimage$/i],
  linux:   [/linux.*\.appimage$/i, /\.appimage$/i, /linux.*\.tar\.gz$/i, /_amd64\.deb$/i],
  deb:     [/_amd64\.deb$/i, /-linux-amd64\.deb$/i],
  arch:    [/\.pkg\.tar\.zst$/i],
  android: [/\.apk$/i],
};
const TTL = 300;

// Fallback when the API is down or rate-limited: the documented v<VERSION>
// asset names (see release.yml / docs/releasing.md), keyed by platform.
// "" means StageWand-<ver>.<ext> with no dash suffix (the DMG).
export const NAMES = {
  mac:      ".dmg",
  windows:  "-windows-x64.zip",
  appimage: "-linux-x86_64.AppImage",
  linux:    "-linux-x86_64.AppImage",
  deb:      "-linux-amd64.deb",
  arch:     "-linux-x86_64.pkg.tar.zst",
  android:  "-android.apk",
};

export function fallbackAsset(tag, platform) {
  const m = /^v(\d+\.\d+\.\d+)$/.exec(tag || "");
  if (!m || !Object.hasOwn(NAMES, platform)) return null;
  const sfx = NAMES[platform];
  const name = `StageWand-${m[1]}${sfx}`;
  return { name, browser_download_url: `https://github.com/${REPO}/releases/download/${tag}/${name}` };
}

export function tagFromLocation(loc) {
  const m = /\/releases\/tag\/(v\d+\.\d+\.\d+)\/?$/.exec(loc || "");
  return m ? m[1] : null;
}

// github.com/<repo>/releases/latest redirects to /releases/tag/<tag>; this
// path is not subject to the API rate limit.
async function latestTag() {
  const r = await fetch(`https://github.com/${REPO}/releases/latest`, {
    method: "HEAD", redirect: "manual", headers: { "User-Agent": "stagewand-site" },
  });
  return tagFromLocation(r.headers.get("Location"));
}

// Unauthenticated GitHub API calls are limited per egress IP, and Cloudflare
// egress IPs are shared, so: fresh copy for 5 min, a stale copy for 7 days to
// serve when the API errors, and an optional GITHUB_TOKEN env var (read-only,
// public repo) to lift the limit.
async function releases(ctx) {
  const cache = caches.default;
  const fresh = new Request(`https://stagewand-cache.internal/releases/${REPO}`);
  const stale = new Request(`https://stagewand-cache.internal/releases-stale/${REPO}`);
  const hit = await cache.match(fresh);
  if (hit) return hit.json();
  const headers = { "User-Agent": "stagewand-site", Accept: "application/vnd.github+json" };
  if (ctx.env && ctx.env.GITHUB_TOKEN) headers.Authorization = `Bearer ${ctx.env.GITHUB_TOKEN}`;
  let r;
  try {
    r = await fetch(`https://api.github.com/repos/${REPO}/releases?per_page=50`, { headers });
  } catch (e) { r = null; }
  if (!r || !r.ok) {
    const e = new Error(`GitHub API ${r ? r.status : "unreachable"}`);
    e.stale = () => cache.match(stale).then((old) => (old ? old.json() : null));
    throw e;
  }
  const body = await r.text();
  const put = (key, ttl) => cache.put(key, new Response(body, {
    headers: { "Content-Type": "application/json", "Cache-Control": `max-age=${ttl}` },
  }));
  ctx.waitUntil(Promise.all([put(fresh, TTL), put(stale, 7 * 86400)]));
  return JSON.parse(body);
}

export function pickAsset(list, patterns) {
  const rels = list.filter((r) => !r.draft && !r.prerelease)
    .sort((x, y) => (y.published_at || "").localeCompare(x.published_at || ""));
  // Pattern order beats recency: the moving `linux` .deb release is
  // republished on every main push and must not shadow a newer-format asset.
  for (const re of patterns) {
    for (const rel of rels) {
      const hit = rel.assets
        .filter((a) => !/\.(sha256|sig|asc|txt)$/i.test(a.name) && a.name !== "SHA256SUMS" && re.test(a.name))
        .sort((a, b) => a.name.length - b.name.length)[0];
      if (hit) return hit;
    }
  }
  return null;
}

export async function onRequest(ctx) {
  const name = String(ctx.params.platform || "").toLowerCase();
  const p = Object.hasOwn(PLATFORMS, name) ? PLATFORMS[name] : null;
  const fallback = new URL(`/?unavailable=${encodeURIComponent(name)}#setup`, ctx.request.url);
  if (!p) return Response.redirect(fallback, 302);
  let asset;
  let stale;
  try { asset = pickAsset(await releases(ctx), p); } catch (e) {
    // API failed: derive the URL from the latest tag, then the stale cache.
    try { asset = fallbackAsset(await latestTag(), name); } catch (_) { asset = null; }
    if (!asset && e.stale) { try { const old = await e.stale(); if (old) asset = pickAsset(old, p); } catch (_) {} }
    else if (asset) stale = e.stale;
    if (!asset) return new Response("Download temporarily unavailable. Please try again shortly.", { status: 503, headers: { "Retry-After": "60" } });
  }
  if (!asset) return Response.redirect(fallback, 302);

  const fetchAsset = (asset) => fetch(asset.browser_download_url, {
    headers: { "User-Agent": "stagewand-site" }, redirect: "follow",
    cf: { cacheEverything: true, cacheTtl: 3600 },
  });
  let upstream = await fetchAsset(asset);
  // A guessed filename may be absent from the latest release. Keep the stale
  // fallback until the guessed download succeeds.
  if ((!upstream.ok || !upstream.body) && stale) {
    let oldAsset;
    try { const old = await stale(); if (old) oldAsset = pickAsset(old, p); } catch (_) {}
    if (oldAsset) {
      if (upstream.body) await upstream.body.cancel();
      asset = oldAsset;
      upstream = await fetchAsset(asset);
    }
  }
  if (!upstream.ok || !upstream.body) {
    return new Response("Download temporarily unavailable.", { status: 502 });
  }
  const h = new Headers({
    "Content-Type": "application/octet-stream",
    "Content-Disposition": `attachment; filename="${asset.name.replace(/"/g, "")}"`,
    "Cache-Control": "public, max-age=300",
    "X-Content-Type-Options": "nosniff",
  });
  const len = upstream.headers.get("Content-Length");
  if (len) h.set("Content-Length", len);
  if (ctx.request.method === "HEAD") return new Response(null, { headers: h });
  return new Response(upstream.body, { headers: h });
}
