# Outtake brand assets

## Origin

The editor mascot is **not** a stock Free Gophers Pack pose. There is no original SVG of this character holding a clapperboard and scissors.

It was generated from [Maria Letta's Free Gophers Pack](https://github.com/MariaLetta/free-gophers-pack) **character 9** (`characters/svg/9.svg` / `characters/png/9.png`, CC0) by compositing film-editor props onto that gopher. `mascot.png` is the source of
truth.

`mascot.svg` is a quantized scanline trace of the isolated raster. It matches the PNG at normal sizes but is not a hand-drawn vector. Prefer `mascot.png` when quality matters.

## Files

| File                                                    | Size                     | Use                                   |
|---------------------------------------------------------|--------------------------|---------------------------------------|
| `mascot.png`                                            | 1024×1024, transparent   | README, docs, print                   |
| `mascot-128.png`                                        | 128×128, transparent     | Sidebar, login, in-app                |
| `mascot.svg`                                            | 512 viewBox              | Optional vector; heavy scanline trace |
| `icon.png`                                              | 1024×1024, charcoal tile | App icon master                       |
| `png/{16,32,48,64,96,128,180,192,256,384,512,1024}.png` | square tiles             | Exact pixel sizes                     |
| `png/512-maskable.png`                                  | 512×512, 20% pad         | PWA maskable                          |
| `favicon.ico`                                           | 16, 32, 48               | Browser tab                           |
| `favicon-16x16.png` / `favicon-32x32.png`               |                          | HTML `rel=icon`                       |
| `apple-touch-icon.png`                                  | 180×180                  | iOS / Safari                          |
| `android-chrome-192x192.png`                            | 192×192                  | Android / PWA                         |
| `android-chrome-512x512.png`                            | 512×512                  | Android / PWA                         |
| `icon-64.png`                                           | 64×64                    | Small app-icon tile                   |
| `og.png`                                                | 1200×630                 | Open Graph / social                   |

The web app embeds a subset under `internal/web/assets/brand/` (served at `/assets/brand/...`).
