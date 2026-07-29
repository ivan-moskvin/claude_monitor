import AppKit
import SwiftUI

struct MenuContent: View {
    @ObservedObject var store: UsageStore

    private let fiveHourBase = Color.green
    private let sevenDayBase = Color.cyan

    var body: some View {
        VStack(spacing: 12) {
            Text("Claude Limit")
                .font(.headline)

            if let fiveHour = store.fiveHour {
                RingStack(
                    outer: fiveHour,
                    outerColor: fiveHour.tint(base: fiveHourBase),
                    inner: store.sevenDay,
                    innerColor: store.sevenDay?.tint(base: sevenDayBase) ?? sevenDayBase
                )

                VStack(spacing: 5) {
                    LegendRow(
                        color: fiveHour.tint(base: fiveHourBase),
                        title: "Сессия · 5 часов",
                        percentage: fiveHour.usedPercentage
                    )

                    if let sevenDay = store.sevenDay {
                        LegendRow(
                            color: sevenDay.tint(base: sevenDayBase),
                            title: "Неделя · 7 дней",
                            percentage: sevenDay.usedPercentage
                        )
                    }
                }

                if let stale = store.staleness {
                    Text(stale)
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                }
            } else if let error = store.errorMessage {
                Text(error)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(height: 150)
            }

            Divider()

            HStack {
                Button("Обновить") { store.refresh() }
                Spacer()
                Button("Выйти") { NSApplication.shared.terminate(nil) }
            }
            .buttonStyle(.borderless)
            .font(.callout)
        }
        .padding(16)
        .frame(width: 250)
    }
}

private struct RingStack: View {
    let outer: UsageWindow
    let outerColor: Color
    let inner: UsageWindow?
    let innerColor: Color

    var body: some View {
        ZStack {
            Ring(
                progress: min(outer.usedPercentage / 100, 1),
                color: outerColor,
                lineWidth: 14
            )
            .frame(width: 152, height: 152)

            if let inner {
                Ring(
                    progress: min(inner.usedPercentage / 100, 1),
                    color: innerColor,
                    lineWidth: 10
                )
                .frame(width: 116, height: 116)
            }

            ClockCenter(countdown: outer.countdown)
        }
        .frame(height: 158)
    }
}

private struct Ring: View {
    let progress: Double
    let color: Color
    let lineWidth: CGFloat

    var body: some View {
        ZStack {
            Circle()
                .stroke(Color.secondary.opacity(0.18), lineWidth: lineWidth)

            Circle()
                .trim(from: 0, to: progress)
                .stroke(color, style: StrokeStyle(lineWidth: lineWidth, lineCap: .round))
                .rotationEffect(.degrees(-90))
                .animation(.easeOut(duration: 0.35), value: progress)
        }
    }
}

private struct ClockCenter: View {
    let countdown: String?

    var body: some View {
        VStack(spacing: 1) {
            Image(systemName: "clock")
                .font(.system(size: 11, weight: .medium))
                .foregroundStyle(.secondary)

            Text(countdown ?? "—")
                .font(.system(size: 26, weight: .semibold, design: .rounded))
                .monospacedDigit()

            Text("до сброса")
                .font(.system(size: 9))
                .foregroundStyle(.secondary)
        }
    }
}

private struct LegendRow: View {
    let color: Color
    let title: String
    let percentage: Double

    var body: some View {
        HStack(spacing: 7) {
            Circle()
                .fill(color)
                .frame(width: 8, height: 8)

            Text(title)
                .font(.caption)

            Spacer()

            Text("\(Int(percentage.rounded()))%")
                .font(.caption.weight(.semibold))
                .monospacedDigit()
        }
    }
}
