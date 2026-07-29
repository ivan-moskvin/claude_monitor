// Окон при старте нет — приложение живёт в строке меню.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    claude_usage_bar_lib::run()
}
