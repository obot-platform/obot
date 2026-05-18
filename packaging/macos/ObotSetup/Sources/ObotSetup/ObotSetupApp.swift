import SwiftUI

@main
struct ObotSetupApp: App {
    var body: some Scene {
        WindowGroup {
            ContentView()
                .frame(minWidth: 620, minHeight: 480)
        }
        .windowResizability(.contentMinSize)
    }
}
