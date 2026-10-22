// Stable download URLs: /download/mac, /download/linux (.deb), /download/arch,
// /download/windows. Resolves the newest matching GitHub release and proxies
// the asset so visitors never land on GitHub.
const REPO = "EdmundLimBoEn/stage-wand";
// Asset matching is by platform keyword + extension, across every release of
// any tag scheme (mac-v*, linux, v*), newest published first. Each platform
// lists patterns in preference order; the first release with any match wins.
export const PLATFORMS = {
  mac:     [/\.dmg$/i],
  windows: [/win(dows)?.*\.(msi|exe|zip)$/i, /\.(msi|exe)$/i],
  linux:   [/linux.*\.appimage$/i, /\.appimage$/i, /linux.*\.tar\.gz$/i, /_amd64\.deb$/i],
  deb:     [/_amd64\.deb$/i],
  arch:    [/\.pkg\.tar\.zst$/i],
  android: [/\.apk$/i],
};
const TTL = 300;

async function releases(ctx) {
  const cache = caches.default;
  const key = new Request(`https://stagewand-cache.internal/releases/${REPO}`);
  const hit = await cache.match(key);
  if (hit) return hit.json();
  const r = await fetch(`https://api.github.com/repos/${REPO}/releases?per_page=50`, {
    headers: { "User-Agent": "stagewand-site", Accept: "application/vnd.github+json" },
  });
  if (!r.ok) throw new Error(`GitHub API ${r.status}`);
  const body = await r.text();
  ctx.waitUntil(cache.put(key, new Response(body, {
    headers: { "Content-Type": "application/json", "Cache-Control": `max-age=${TTL}` },
  })));
  return JSON.parse(body);
}

export function pickAsset(list, patterns) {
  const rels = list.filter((r) => !r.draft && !r.prerelease)
    .sort((x, y) => (y.published_at || "").localeCompare(x.published_at || ""));
  for (const rel of rels) {
    const names = rel.assets.filter((a) => !/\.(sha256|sig|asc|txt)$/i.test(a.name) && a.name !== "SHA256SUMS");
    for (const re of patterns) {
      // Shortest name first: prefers stable unversioned aliases when present.
      const hit = names.filter((a) => re.test(a.name)).sort((a, b) => a.name.length - b.name.length)[0];
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
  try { asset = pickAsset(await releases(ctx), p); } catch (e) {
    return new Response("Download temporarily unavailable. Please try again shortly.", { status: 503, headers: { "Retry-After": "60" } });
  }
  if (!asset) return Response.redirect(fallback, 302);

  const upstream = await fetch(asset.browser_download_url, {
    headers: { "User-Agent": "stagewand-site" }, redirect: "follow",
    cf: { cacheEverything: true, cacheTtl: 3600 },
  });
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
