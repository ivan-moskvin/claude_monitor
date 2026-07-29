//! Отрисовка иконки в строке меню: кольцо расхода 5-часового окна.
//!
//! Рисуем в RGBA-буфер через tiny-skia — WebView для иконки размером
//! со строку меню не поднять, а системного API у Tauri для этого нет.

use crate::snapshot::Level;
use std::f32::consts::PI;

/// Рисуем с запасом по плотности: система ужимает иконку под высоту
/// строки меню, и на retina запас превращается в резкость.
const CANVAS: u32 = 44;
const LINE_WIDTH: f32 = 6.4;

pub fn menu_bar_icon(progress: Option<f32>, level: Level) -> Vec<u8> {
    let mut pixmap = tiny_skia::Pixmap::new(CANVAS, CANVAS).expect("ненулевой размер холста");

    let center = CANVAS as f32 / 2.0;
    let radius = center - LINE_WIDTH / 2.0 - 1.0;

    let stroke = tiny_skia::Stroke {
        width: LINE_WIDTH,
        line_cap: tiny_skia::LineCap::Round,
        ..Default::default()
    };

    let track_stroke = tiny_skia::Stroke {
        width: LINE_WIDTH,
        ..Default::default()
    };

    // Дорожка кольца — нейтральный серый, читается и на светлой, и на тёмной теме.
    if let Some(track) = full_circle(center, radius) {
        pixmap.stroke_path(
            &track,
            &paint(128, 128, 128, 110),
            &track_stroke,
            tiny_skia::Transform::identity(),
            None,
        );
    }

    match progress {
        // Данных нет — точка в центре вместо дуги.
        None => {
            if let Some(dot) = full_circle(center, LINE_WIDTH * 0.6) {
                pixmap.fill_path(
                    &dot,
                    &paint(128, 128, 128, 150),
                    tiny_skia::FillRule::Winding,
                    tiny_skia::Transform::identity(),
                    None,
                );
            }
        }
        Some(progress) if progress > 0.0 => {
            let (r, g, b) = level_color(level);
            let alpha = if level == Level::Expired { 110 } else { 255 };

            if let Some(arc) = arc(center, radius, progress.min(1.0)) {
                pixmap.stroke_path(
                    &arc,
                    &paint(r, g, b, alpha),
                    &stroke,
                    tiny_skia::Transform::identity(),
                    None,
                );
            }
        }
        Some(_) => {}
    }

    pixmap.take()
}

pub const ICON_SIZE: u32 = CANVAS;

/// Цвета акцентов macOS — они одинаково читаются на обеих темах.
fn level_color(level: Level) -> (u8, u8, u8) {
    match level {
        Level::Ok => (52, 199, 89),
        Level::Warn => (255, 149, 0),
        Level::Critical => (255, 59, 48),
        Level::Expired => (142, 142, 147),
    }
}

fn paint<'a>(r: u8, g: u8, b: u8, a: u8) -> tiny_skia::Paint<'a> {
    let mut paint = tiny_skia::Paint::default();
    paint.set_color_rgba8(r, g, b, a);
    paint.anti_alias = true;
    paint
}

fn full_circle(center: f32, radius: f32) -> Option<tiny_skia::Path> {
    let mut builder = tiny_skia::PathBuilder::new();
    builder.push_circle(center, center, radius);
    builder.finish()
}

/// Дуга расхода: от 12 часов по часовой стрелке.
fn arc(center: f32, radius: f32, progress: f32) -> Option<tiny_skia::Path> {
    let sweep = 2.0 * PI * progress;
    let start = -PI / 2.0;

    let mut builder = tiny_skia::PathBuilder::new();
    builder.move_to(center + radius * start.cos(), center + radius * start.sin());

    // Кубическая аппроксимация точна на четверти круга — режем дугу на такие куски.
    let segments = (progress * 4.0).ceil().max(1.0) as usize;
    let step = sweep / segments as f32;

    for index in 0..segments {
        let from = start + step * index as f32;
        let to = from + step;
        // Классический коэффициент кубической аппроксимации дуги.
        let handle = 4.0 / 3.0 * (step / 4.0).tan();

        let (from_cos, from_sin) = (from.cos(), from.sin());
        let (to_cos, to_sin) = (to.cos(), to.sin());

        builder.cubic_to(
            center + radius * (from_cos - handle * from_sin),
            center + radius * (from_sin + handle * from_cos),
            center + radius * (to_cos + handle * to_sin),
            center + radius * (to_sin - handle * to_cos),
            center + radius * to_cos,
            center + radius * to_sin,
        );
    }

    builder.finish()
}
