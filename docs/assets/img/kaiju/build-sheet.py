#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# © The Shinari Authors
#
# Pack the kaiju frames in this directory into ../kaiju-sprites.png (the sheet
# the hero animation loads). Run it after correcting any frame:
#
#     python build-sheet.py
#
# It prints the CELL_W / CELL_H / GUTTER / MOUTH_FX / MOUTH_FY values; paste any
# that changed into docs/assets/js/hero-grid.js.
#
# A transparent GUTTER between frames stops edge-bleed: when the sprite is drawn
# at sub-pixel positions the scaler can sample just past a frame, and without
# the gutter it would pick up the neighbour frame's (white-outlined) edge as a
# thin vertical line.
import os
from PIL import Image

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, "..", "kaiju-sprites.png")
GUTTER = 8

ORDER = [
    "01-sprite-idle-chest-dull.png",
    "02-sprite-idle-pulse1.png",
    "03-sprite-idle-chest-flare.png",
    "04-sprite-idle-chest-dull.png",
    "05-sprite-idle-tail-twitch.png",
    "sprite-walk-A.png",
    "sprite-walk-B.png",
    "sprite-charge.png",
    "sprite-fire-no-laser.png",
]

imgs = [Image.open(os.path.join(HERE, n)).convert("RGBA") for n in ORDER]
W, H = imgs[0].size

# union bounding box across every frame keeps all poses aligned on one canvas
ux0 = uy0 = 10**9
ux1 = uy1 = -1
for im in imgs:
    bb = im.getbbox()
    if bb:
        ux0 = min(ux0, bb[0])
        uy0 = min(uy0, bb[1])
        ux1 = max(ux1, bb[2])
        uy1 = max(uy1, bb[3])
crop = (ux0, uy0, ux1, uy1)
frames = [im.crop(crop) for im in imgs]
cw, chh = frames[0].size

sheet = Image.new("RGBA", (cw * len(frames) + GUTTER * (len(frames) - 1), chh), (0, 0, 0, 0))
for i, f in enumerate(frames):
    sheet.paste(f, (i * (cw + GUTTER), 0))
sheet.save(OUT)

# beam origin (mouth) measured from the with-laser fire frame
fire = Image.open(os.path.join(HERE, "sprite-fire.png")).convert("RGBA")
fp = fire.load()


def bright(x, y):
    r, g, b, a = fp[x, y]
    return a > 40 and (r + g + b) // 3 > 150


rows = {}
for y in range(H):
    c = sum(1 for x in range(600, W) if bright(x, y))
    if c > 5:
        rows[y] = c
beam_y = int(sum(k * v for k, v in rows.items()) / sum(rows.values()))
mouth_x = W
for y in range(beam_y - 8, beam_y + 9):
    gap = 0
    last = W - 1
    for x in range(W - 1, 150, -1):
        if bright(x, y):
            gap = 0
            last = x
        else:
            gap += 1
            if gap > 10:
                break
    mouth_x = min(mouth_x, last)

print("wrote", os.path.relpath(OUT, HERE))
print("CELL_W   =", cw)
print("CELL_H   =", chh)
print("GUTTER   =", GUTTER)
print("MOUTH_FX = %.3f" % ((mouth_x - crop[0]) / cw))
print("MOUTH_FY = %.3f" % ((beam_y - crop[1]) / chh))
