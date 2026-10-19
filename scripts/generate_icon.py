from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter


ROOT = Path(__file__).resolve().parent.parent
BUILD = ROOT / "build"
SCALE = 2
SIZE = 1024 * SCALE


def s(value):
    return int(round(value * SCALE))


def rgba(hex_color, alpha=255):
    value = hex_color.lstrip("#")
    return tuple(int(value[i : i + 2], 16) for i in (0, 2, 4)) + (alpha,)


def rounded_rect(draw, box, radius, fill=None, outline=None, width=1):
    draw.rounded_rectangle(
        tuple(s(value) for value in box),
        radius=s(radius),
        fill=fill,
        outline=outline,
        width=s(width),
    )


def main():
    image = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
    draw = ImageDraw.Draw(image)

    rounded_rect(draw, (0, 0, 1024, 1024), 220, fill=rgba("#20242b"))
    rounded_rect(
        draw,
        (8, 8, 1016, 1016),
        214,
        outline=rgba("#ffffff", 18),
        width=8,
    )

    shadow = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
    shadow_draw = ImageDraw.Draw(shadow)
    rounded_rect(
        shadow_draw,
        (246, 188, 794, 892),
        34,
        fill=rgba("#000000", 66),
    )
    shadow = shadow.filter(ImageFilter.GaussianBlur(s(18)))
    image = Image.alpha_composite(image, shadow)
    draw = ImageDraw.Draw(image)

    rounded_rect(draw, (238, 170, 786, 874), 34, fill=rgba("#f7f3ea"))
    draw.polygon(
        [(s(696), s(170)), (s(786), s(260)), (s(696), s(260))],
        fill=rgba("#e2d9cb"),
    )
    draw.line(
        [(s(696), s(170)), (s(786), s(260))],
        fill=rgba("#d4c8b6"),
        width=s(5),
    )

    rounded_rect(draw, (330, 382, 630, 406), 12, fill=rgba("#262b33", 204))
    rounded_rect(draw, (330, 446, 580, 470), 12, fill=rgba("#262b33", 158))
    rounded_rect(draw, (330, 510, 610, 534), 12, fill=rgba("#262b33", 112))

    start = (s(372), s(724))
    end = (s(690), s(420))
    draw.line([start, end], fill=rgba("#c8553d"), width=s(88))
    radius = s(44)
    for x, y in (start, end):
        draw.ellipse(
            (x - radius, y - radius, x + radius, y + radius),
            fill=rgba("#c8553d"),
        )
    draw.line([start, end], fill=rgba("#e0785f", 140), width=s(18))
    highlight_radius = s(9)
    for x, y in (start, end):
        draw.ellipse(
            (
                x - highlight_radius,
                y - highlight_radius,
                x + highlight_radius,
                y + highlight_radius,
            ),
            fill=rgba("#e0785f", 140),
        )

    icon = image.resize((1024, 1024), Image.Resampling.LANCZOS)
    BUILD.mkdir(parents=True, exist_ok=True)
    icon.save(BUILD / "appicon.png")

    windows_dir = BUILD / "windows"
    windows_dir.mkdir(parents=True, exist_ok=True)
    icon.save(
        windows_dir / "icon.ico",
        format="ICO",
        sizes=[
            (16, 16),
            (24, 24),
            (32, 32),
            (48, 48),
            (64, 64),
            (128, 128),
            (256, 256),
        ],
    )

    print("Generated build/appicon.png and build/windows/icon.ico")


if __name__ == "__main__":
    main()
