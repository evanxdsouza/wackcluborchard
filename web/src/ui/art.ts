// Generated art: the pixel-sky project banners, pixel avatars and the
// Wack Club Orchard mark. Everything is drawn from a seed so a project
// always gets the same sky.

import { h, useEffect, useRef } from "../lib/sprout.js";

function rng(seed: string) {
  let x = 2166136261;
  for (let i = 0; i < seed.length; i++) x = Math.imul(x ^ seed.charCodeAt(i), 16777619);
  return () => {
    x ^= x << 13;
    x ^= x >>> 17;
    x ^= x << 5;
    return ((x >>> 0) % 100000) / 100000;
  };
}

type RGB = [number, number, number];

interface Palette {
  sky: RGB[]; // top to horizon
  cloud: RGB[]; // shadow to highlight
  ground: RGB[]; // horizon to bottom
  groundKind: "sea" | "hills" | "city";
  stars?: boolean;
  sun?: RGB;
}

export const palettes: Record<string, Palette> = {
  clouds: {
    sky: [[110, 150, 214], [140, 175, 228], [170, 198, 238], [198, 216, 243], [214, 226, 245]],
    cloud: [[168, 190, 226], [196, 212, 238], [226, 234, 248], [246, 249, 253]],
    ground: [[154, 170, 196], [118, 134, 162], [92, 106, 134], [66, 78, 104], [48, 58, 82]],
    groundKind: "sea",
  },
  dusk: {
    sky: [[58, 44, 102], [118, 64, 128], [196, 96, 120], [240, 150, 110], [252, 196, 140]],
    cloud: [[140, 76, 118], [196, 108, 124], [236, 150, 136], [252, 196, 168]],
    ground: [[120, 76, 110], [88, 56, 96], [64, 42, 80], [44, 30, 62], [30, 22, 46]],
    groundKind: "sea",
    sun: [255, 214, 150],
  },
  meadow: {
    sky: [[120, 186, 232], [150, 204, 238], [182, 220, 242], [210, 232, 244], [226, 240, 246]],
    cloud: [[190, 214, 232], [214, 230, 242], [236, 244, 250], [252, 253, 255]],
    ground: [[146, 196, 108], [118, 172, 86], [92, 146, 70], [70, 120, 58], [52, 96, 48]],
    groundKind: "hills",
  },
  wackcluborchard: {
    sky: [[238, 170, 150], [244, 190, 160], [248, 208, 172], [250, 222, 186], [252, 234, 204]],
    cloud: [[232, 170, 160], [242, 196, 178], [250, 220, 200], [255, 240, 226]],
    ground: [[176, 72, 76], [150, 54, 62], [122, 42, 52], [96, 34, 44], [72, 26, 36]],
    groundKind: "hills",
    sun: [255, 244, 214],
  },
  night: {
    sky: [[10, 14, 34], [16, 22, 52], [26, 34, 72], [38, 48, 92], [52, 62, 108]],
    cloud: [[34, 42, 78], [46, 56, 96], [62, 72, 114], [80, 90, 132]],
    ground: [[30, 36, 66], [22, 28, 54], [16, 20, 42], [12, 14, 32], [8, 10, 24]],
    groundKind: "city",
    stars: true,
  },
  sunrise: {
    sky: [[136, 170, 216], [196, 190, 210], [240, 196, 180], [252, 214, 170], [254, 232, 190]],
    cloud: [[210, 180, 190], [236, 204, 196], [250, 226, 212], [255, 244, 232]],
    ground: [[120, 142, 170], [96, 118, 148], [74, 94, 124], [56, 74, 102], [40, 56, 82]],
    groundKind: "sea",
    sun: [255, 238, 200],
  },
  ocean: {
    sky: [[64, 150, 196], [96, 176, 212], [132, 200, 226], [170, 220, 236], [200, 234, 242]],
    cloud: [[160, 206, 226], [196, 226, 238], [228, 242, 248], [250, 253, 255]],
    ground: [[60, 150, 180], [40, 124, 160], [28, 100, 138], [20, 78, 114], [14, 58, 90]],
    groundKind: "sea",
  },
  lavender: {
    sky: [[132, 120, 196], [160, 146, 212], [188, 174, 226], [212, 200, 238], [230, 222, 246]],
    cloud: [[176, 162, 214], [202, 190, 230], [228, 220, 244], [248, 244, 253]],
    ground: [[150, 130, 190], [122, 104, 166], [98, 82, 142], [76, 62, 118], [56, 46, 94]],
    groundKind: "hills",
  },
};

function noise2(r: () => number, w: number, hgt: number, scale: number) {
  const gw = Math.ceil(w / scale) + 2;
  const gh = Math.ceil(hgt / scale) + 2;
  const g: number[] = [];
  for (let i = 0; i < gw * gh; i++) g.push(r());
  const smooth = (t: number) => t * t * (3 - 2 * t);
  return (x: number, y: number) => {
    const gx = x / scale, gy = y / scale;
    const x0 = Math.floor(gx), y0 = Math.floor(gy);
    const tx = smooth(gx - x0), ty = smooth(gy - y0);
    const v = (i: number, j: number) => g[(j % gh) * gw + (i % gw)];
    const a = v(x0, y0) * (1 - tx) + v(x0 + 1, y0) * tx;
    const b = v(x0, y0 + 1) * (1 - tx) + v(x0 + 1, y0 + 1) * tx;
    return a * (1 - ty) + b * ty;
  };
}

const bayer = [
  [0, 8, 2, 10],
  [12, 4, 14, 6],
  [3, 11, 1, 9],
  [15, 7, 13, 5],
].map((r) => r.map((v) => v / 16));

function pick(stops: RGB[], t: number, x: number, y: number): RGB {
  // ordered dithering between adjacent bands gives the pixel-art look
  const f = Math.max(0, Math.min(0.9999, t)) * (stops.length - 1);
  const i = Math.floor(f);
  const frac = f - i;
  return frac > bayer[y & 3][x & 3] ? stops[Math.min(i + 1, stops.length - 1)] : stops[i];
}

export function drawSky(canvas: HTMLCanvasElement, seed: string, preset: string) {
  const pal = palettes[preset] || palettes.clouds;
  const W = 320, H = 92;
  canvas.width = W;
  canvas.height = H;
  const ctx = canvas.getContext("2d")!;
  const img = ctx.createImageData(W, H);
  const r = rng(seed + preset);
  const n1 = noise2(r, W, H, 22);
  const n2 = noise2(r, W, H, 9);
  const n3 = noise2(r, W, H, 4);
  const horizon = Math.floor(H * 0.62);
  const sunX = Math.floor(W * (0.2 + r() * 0.6));
  const sunY = Math.floor(horizon * 0.55);
  for (let y = 0; y < H; y++) {
    for (let x = 0; x < W; x++) {
      let c: RGB;
      if (y < horizon) {
        c = pick(pal.sky, y / horizon, x, y);
        if (pal.stars && r() < 0.012 && y < horizon * 0.8) c = [230, 232, 255];
        if (pal.sun) {
          const d = Math.hypot((x - sunX) * 0.9, y - sunY);
          if (d < 7) c = pal.sun;
          else if (d < 11 && (x + y) % 2 === 0) c = pal.sun;
        }
        // clouds: layered noise, denser towards the horizon band
        const band = 1 - Math.abs((y / horizon) - 0.55) * 1.6;
        const v = n1(x, y * 2.2) * 0.6 + n2(x, y * 2) * 0.3 + n3(x, y) * 0.1;
        const density = v + band * 0.28 - 0.62;
        if (density > 0) {
          const lvl = Math.min(1, density * 4.5 + (1 - y / horizon) * 0.25);
          c = pick(pal.cloud, lvl, x, y);
        }
      } else {
        const t = (y - horizon) / (H - horizon);
        if (pal.groundKind === "sea") {
          c = pick(pal.ground, t, x, y);
          // wave glints
          const wv = n3(x * 1.5, y * 6);
          if (wv > 0.72 && (x + y) % 3 !== 0) c = pal.ground[Math.max(0, Math.floor(t * (pal.ground.length - 1)) - 1)];
          if (y === horizon) c = pal.ground[0];
        } else if (pal.groundKind === "hills") {
          const hill = horizon - 8 + Math.floor(n1(x * 1.8, 5) * 14);
          c = pick(pal.ground, t, x, y);
          if (y < hill + 2) c = pal.ground[0];
        } else {
          c = pick(pal.ground, t, x, y);
          // lit windows
          if ((x % 7 === 2 || x % 11 === 5) && (y % 4 === 1) && n2(x, y) > 0.55) c = [250, 214, 120];
        }
      }
      const o = (y * W + x) * 4;
      img.data[o] = c[0];
      img.data[o + 1] = c[1];
      img.data[o + 2] = c[2];
      img.data[o + 3] = 255;
    }
  }
  // hills and cities cut into the sky
  if (pal.groundKind !== "sea") {
    for (let x = 0; x < W; x++) {
      const top = pal.groundKind === "hills"
        ? horizon - 6 - Math.floor(n1(x * 1.3, 3) * 18)
        : horizon - (Math.floor(x / 9) % 3 === 0 ? 0 : 4 + Math.floor(n1(Math.floor(x / 9) * 9, 1) * 26));
      for (let y = Math.max(0, top); y < horizon; y++) {
        const c = pal.groundKind === "hills" ? pick(pal.ground, (y - top) / 30, x, y) : pal.ground[1];
        let cc = c;
        if (pal.groundKind === "city" && (x % 4 === 1) && (y % 4 === 2) && n2(x, y) > 0.5) cc = [250, 214, 120];
        const o = (y * W + x) * 4;
        img.data[o] = cc[0];
        img.data[o + 1] = cc[1];
        img.data[o + 2] = cc[2];
      }
    }
  }
  ctx.putImageData(img, 0, 0);
}

export function Sky({ seed, preset, class: cls }: { seed: string; preset: string; class?: string }) {
  const ref = useRef<HTMLCanvasElement | null>(null);
  useEffect(() => {
    if (ref.current) drawSky(ref.current, seed, preset);
  }, [seed, preset]);
  return h("canvas", { ref, class: "sky " + (cls || "") });
}

/** A small symmetric pixel critter for avatars. */
export function avatarSVG(seed: string, size = 28): string {
  const r = rng(seed || "wackcluborchard");
  const hues = [350, 20, 40, 150, 190, 220, 265, 300];
  const hue = hues[Math.floor(r() * hues.length)];
  const fg = `hsl(${hue} 70% 48%)`;
  const fg2 = `hsl(${(hue + 30) % 360} 80% 62%)`;
  const bg = `hsl(${hue} 60% 94%)`;
  let rects = "";
  for (let y = 0; y < 8; y++) {
    for (let x = 0; x < 4; x++) {
      const on = y > 0 && y < 7 && (r() > 0.45 || (x === 3 && y > 1 && y < 6));
      if (!on) continue;
      const c = r() > 0.75 ? fg2 : fg;
      rects += `<rect x="${x}" y="${y}" width="1" height="1" fill="${c}"/><rect x="${7 - x}" y="${y}" width="1" height="1" fill="${c}"/>`;
    }
  }
  // eyes
  const ey = 2 + Math.floor(r() * 2);
  rects += `<rect x="2" y="${ey}" width="1" height="1" fill="#fff"/><rect x="5" y="${ey}" width="1" height="1" fill="#fff"/>`;
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="-1 -1 10 10" width="${size}" height="${size}" shape-rendering="crispEdges"><rect x="-1" y="-1" width="10" height="10" fill="${bg}"/>${rects}</svg>`;
}

export function Avatar({ seed, size = 28, title }: { seed: string; size?: number; title?: string }) {
  return h("span", { class: "avatar", style: { width: size, height: size }, title, innerHTML: avatarSVG(seed, size) });
}

/** The Wack Club Orchard mark: a red tile with a cluster of fruit. */
export const logoSVG = `<svg viewBox="0 0 40 40" xmlns="http://www.w3.org/2000/svg" aria-hidden="true"><defs><linearGradient id="wco-g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#e5484d"/><stop offset="1" stop-color="#a8232d"/></linearGradient></defs><rect width="40" height="40" rx="11" fill="url(#wco-g)"/><g fill="none" stroke="#fff" stroke-width="3.2"><circle cx="15.2" cy="16" r="5.6"/><circle cx="25.6" cy="18.6" r="5.6"/><circle cx="17.4" cy="26.4" r="5.6"/></g><path d="M22 7.5c2.6.2 4.2 1.6 4.8 4" stroke="#fff" stroke-width="2.6" stroke-linecap="round" fill="none"/></svg>`;

export function Logo({ size = 34 }: { size?: number }) {
  return h("span", { class: "logo", style: { width: size, height: size }, innerHTML: logoSVG });
}

/** Brand-ish badge for well-known images, else a letter tile. */
export function ImageBadge({ image, name, size = 36 }: { image: string; name: string; size?: number }) {
  const img = (image || "").toLowerCase();
  const known: [RegExp, string, string, string][] = [
    [/nginx/, "#0b9444", "#e8f6ee", "NGINX"],
    [/redis/, "#d82c20", "#fdecea", "redis"],
    [/umami/, "#111", "#eee", "um"],
    [/postgres/, "#336791", "#e8eff6", "PG"],
    [/node/, "#3c873a", "#eaf4ea", "JS"],
    [/python/, "#3776ab", "#e9f0f8", "Py"],
    [/traefik|whoami/, "#24a1c1", "#e6f5f9", "tr"],
    [/uptime-kuma/, "#5cdd8b", "#eafaf0", "UK"],
    [/umami/, "#111", "#eee", "um"],
    [/n8n/, "#ea4b71", "#fdebf0", "n8n"],
    [/vaultwarden|bitwarden/, "#175ddc", "#e8effc", "VW"],
    [/gitea/, "#609926", "#eef5e6", "gt"],
    [/ghost/, "#15171a", "#ececec", "G"],
    [/minio/, "#c72c48", "#fbe9ed", "S3"],
    [/metabase/, "#509ee3", "#eaf3fc", "MB"],
    [/excalidraw/, "#6965db", "#eeedfb", "Ex"],
  ];
  for (const [re, fg, bg, label] of known) {
    if (re.test(img)) {
      return h("span", { class: "badge-tile", style: { width: size, height: size, color: fg, background: bg, fontSize: Math.round(size * (label.length > 4 ? 0.2 : label.length > 3 ? 0.24 : label.length > 2 ? 0.28 : 0.34)) } }, label);
    }
  }
  return h("span", { class: "badge-tile letter", style: { width: size, height: size } }, (name || "?")[0].toUpperCase());
}
