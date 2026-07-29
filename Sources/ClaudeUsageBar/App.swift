import SwiftUI

@main
struct ClaudeUsageBarApp: App {
    @StateObject private var store = UsageStore()

    var body: some Scene {
        MenuBarExtra {
            MenuContent(store: store)
        } label: {
            Image(nsImage: store.barImage)
                .task { store.start() }
        }
        .menuBarExtraStyle(.window)
    }
}
