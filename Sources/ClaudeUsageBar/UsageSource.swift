import Foundation

struct UsageWindow: Identifiable {
    let id: String
    let title: String
    let usedPercentage: Double
    let resetsAt: Date?

    var remainingPercentage: Double { max(0, 100 - usedPercentage) }
}

struct UsageSnapshot {
    let windows: [UsageWindow]
    let updatedAt: Date?

    func window(_ id: String) -> UsageWindow? {
        windows.first { $0.id == id }
    }
}

enum UsageSourceError: LocalizedError {
    case missing
    case unreadable
    case empty

    var errorDescription: String? {
        switch self {
        case .missing, .empty:
            return "Запустите сессию Claude Code для получения данных"
        case .unreadable:
            return "Файл со снапшотом повреждён"
        }
    }
}

enum UsageSource {
    static let snapshotURL = FileManager.default
        .homeDirectoryForCurrentUser
        .appendingPathComponent(".claude/usage-snapshot.json")

    private static let knownWindows: [(key: String, title: String)] = [
        ("five_hour", "Сессия · 5 часов"),
        ("seven_day", "Неделя"),
        ("seven_day_opus", "Неделя · Opus"),
    ]

    static func read() throws -> UsageSnapshot {
        guard FileManager.default.fileExists(atPath: snapshotURL.path) else {
            throw UsageSourceError.missing
        }
        guard let data = try? Data(contentsOf: snapshotURL) else {
            throw UsageSourceError.unreadable
        }
        guard let root = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            throw UsageSourceError.unreadable
        }

        let limits = root["rate_limits"] as? [String: Any] ?? [:]

        let windows: [UsageWindow] = knownWindows.compactMap { known in
            guard
                let entry = limits[known.key] as? [String: Any],
                let percentage = number(entry["used_percentage"])
            else { return nil }

            return UsageWindow(
                id: known.key,
                title: known.title,
                usedPercentage: percentage,
                resetsAt: date(entry["resets_at"])
            )
        }

        guard !windows.isEmpty else { throw UsageSourceError.empty }

        return UsageSnapshot(
            windows: windows,
            updatedAt: date(root["updated_at"])
        )
    }

    private static func number(_ value: Any?) -> Double? {
        if let double = value as? Double { return double }
        if let int = value as? Int { return Double(int) }
        return nil
    }

    private static func date(_ value: Any?) -> Date? {
        if let seconds = number(value) {
            return Date(timeIntervalSince1970: seconds > 1_000_000_000_000 ? seconds / 1000 : seconds)
        }
        if let text = value as? String {
            let withFraction = ISO8601DateFormatter()
            withFraction.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            let plain = ISO8601DateFormatter()
            plain.formatOptions = [.withInternetDateTime]
            return withFraction.date(from: text) ?? plain.date(from: text)
        }
        return nil
    }

}
