import AppKit
import Foundation
import SwiftUI

@MainActor
final class UsageStore: ObservableObject {
    @Published private(set) var snapshot: UsageSnapshot?
    @Published private(set) var errorMessage: String?

    private var timer: Timer?
    private let refreshInterval: TimeInterval = 15

    func start() {
        guard timer == nil else { return }
        refresh()
        timer = Timer.scheduledTimer(withTimeInterval: refreshInterval, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.refresh() }
        }
    }

    func refresh() {
        do {
            snapshot = try UsageSource.read()
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    var fiveHour: UsageWindow? { snapshot?.window("five_hour") }
    var sevenDay: UsageWindow? { snapshot?.window("seven_day") }

    var staleness: String? {
        guard let updatedAt = snapshot?.updatedAt else { return nil }
        let age = Date().timeIntervalSince(updatedAt)
        if age < 90 { return nil }

        let formatter = RelativeDateTimeFormatter()
        formatter.unitsStyle = .full
        formatter.locale = Locale(identifier: "ru_RU")
        return "данные \(formatter.localizedString(for: updatedAt, relativeTo: Date()))"
    }

    var barImage: NSImage {
        RingRenderer.menuBarRing(
            progress: fiveHour.map { min($0.usedPercentage / 100, 1) },
            color: NSColor(fiveHour?.tint ?? .secondary),
            showsAlert: fiveHour == nil
        )
    }
}

extension UsageWindow {
    var tint: Color { tint(base: .green) }

    func tint(base: Color) -> Color {
        switch usedPercentage {
        case ..<60: return base
        case ..<85: return .orange
        default: return .red
        }
    }

    var countdown: String? {
        guard let resetsAt else { return nil }
        let seconds = Int(resetsAt.timeIntervalSinceNow)
        if seconds <= 0 { return "0:00" }
        return String(format: "%d:%02d", seconds / 3600, (seconds % 3600) / 60)
    }
}

enum RingRenderer {
    static func menuBarRing(progress: Double?, color: NSColor, showsAlert: Bool) -> NSImage {
        let size = NSSize(width: 18, height: 18)
        let image = NSImage(size: size, flipped: false) { rect in
            let lineWidth: CGFloat = 2.6
            let bounds = rect.insetBy(dx: lineWidth / 2 + 0.5, dy: lineWidth / 2 + 0.5)
            let center = NSPoint(x: bounds.midX, y: bounds.midY)
            let radius = min(bounds.width, bounds.height) / 2

            let track = NSBezierPath()
            track.appendArc(withCenter: center, radius: radius, startAngle: 0, endAngle: 360)
            track.lineWidth = lineWidth
            NSColor.tertiaryLabelColor.setStroke()
            track.stroke()

            if showsAlert {
                let dot = NSBezierPath(
                    ovalIn: NSRect(x: center.x - 1.6, y: center.y - 1.6, width: 3.2, height: 3.2)
                )
                NSColor.tertiaryLabelColor.setFill()
                dot.fill()
                return true
            }

            if let progress, progress > 0 {
                let arc = NSBezierPath()
                arc.appendArc(
                    withCenter: center,
                    radius: radius,
                    startAngle: 90,
                    endAngle: 90 - 360 * progress,
                    clockwise: true
                )
                arc.lineWidth = lineWidth
                arc.lineCapStyle = .round
                color.setStroke()
                arc.stroke()
            }

            return true
        }
        image.isTemplate = false
        return image
    }
}
