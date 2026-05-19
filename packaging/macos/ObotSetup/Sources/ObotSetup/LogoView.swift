import SwiftUI

struct LogoView: View {
    var body: some View {
        if let image = loadLogoImage() {
            Image(nsImage: image)
                .resizable()
                .scaledToFit()
                .accessibilityLabel("Obot")
        } else {
            Text("Obot")
                .font(.system(size: 28, weight: .semibold))
                .foregroundStyle(Color(red: 0.31, green: 0.49, blue: 0.95))
        }
    }

    private func loadLogoImage() -> NSImage? {
        guard let url = Bundle.module.url(
            forResource: "obot-icon-blue",
            withExtension: "svg"
        ) else {
            return nil
        }

        return NSImage(contentsOf: url)
    }
}
