import ImageIO
import SwiftUI
import UIKit
import WebKit

func copyJPEG(_ data: Data) throws -> Data {
  guard let source = CGImageSourceCreateWithData(data as CFData, nil),
    let image = CGImageSourceCreateThumbnailAtIndex(
      source, 0,
      [
        kCGImageSourceCreateThumbnailFromImageAlways: true,
        kCGImageSourceCreateThumbnailWithTransform: true, kCGImageSourceThumbnailMaxPixelSize: 2400,
      ] as CFDictionary),
    let bytes = UIImage(cgImage: image).jpegData(compressionQuality: 0.85),
    bytes.count <= 8 * 1024 * 1024
  else { throw AppError.invalidPhoto }
  return bytes
}
struct Camera: UIViewControllerRepresentable {
  let done: (UIImage?) -> Void
  final class Coordinator: NSObject, UIImagePickerControllerDelegate, UINavigationControllerDelegate
  {
    let done: (UIImage?) -> Void
    init(_ done: @escaping (UIImage?) -> Void) { self.done = done }
    func imagePickerControllerDidCancel(_ picker: UIImagePickerController) { done(nil) }
    func imagePickerController(
      _ picker: UIImagePickerController,
      didFinishPickingMediaWithInfo info: [UIImagePickerController.InfoKey: Any]
    ) { done(info[.originalImage] as? UIImage) }
  }
  func makeCoordinator() -> Coordinator { Coordinator(done) }
  func makeUIViewController(context: Context) -> UIImagePickerController {
    let vc = UIImagePickerController()
    vc.sourceType = .camera
    vc.delegate = context.coordinator
    return vc
  }
  func updateUIViewController(_ uiViewController: UIImagePickerController, context: Context) {}
}
struct ShareFile: UIViewControllerRepresentable {
  let url: URL
  func makeUIViewController(context: Context) -> UIActivityViewController {
    UIActivityViewController(activityItems: [url], applicationActivities: nil)
  }
  func updateUIViewController(_ uiViewController: UIActivityViewController, context: Context) {}
}

struct FormulaText: View {
  let value: String
  @State private var height: CGFloat = 64
  var body: some View {
    if value.contains("$") {
      MathPage(value: value, height: $height).frame(height: height)
    } else {
      Text(value).textSelection(.enabled)
    }
  }
}
private struct MathPage: UIViewRepresentable {
  let value: String
  @Binding var height: CGFloat
  final class Coordinator: NSObject, WKScriptMessageHandler {
    var height: Binding<CGFloat>
    var previous: String?
    init(_ binding: Binding<CGFloat>) { height = binding }
    func userContentController(
      _ controller: WKUserContentController, didReceive message: WKScriptMessage
    ) { if let n = message.body as? NSNumber { height.wrappedValue = max(24, n.doubleValue + 2) } }
  }
  func makeCoordinator() -> Coordinator { Coordinator($height) }
  func makeUIView(context: Context) -> WKWebView {
    let cfg = WKWebViewConfiguration()
    cfg.websiteDataStore = .nonPersistent()
    cfg.userContentController.add(context.coordinator, name: "size")
    let view = WKWebView(frame: .zero, configuration: cfg)
    view.scrollView.isScrollEnabled = false
    view.isOpaque = false
    view.backgroundColor = .clear
    return view
  }
  func updateUIView(_ view: WKWebView, context: Context) {
    guard context.coordinator.previous != value else { return }
    context.coordinator.previous = value
    guard
      let script = Bundle.main.url(forResource: "katex.min", withExtension: "txt").flatMap({
        try? String(contentsOf: $0, encoding: .utf8)
      }),
      let template = Bundle.main.url(forResource: "math", withExtension: "html").flatMap({
        try? String(contentsOf: $0, encoding: .utf8)
      })
    else { return }
    let html = template.replacingOccurrences(of: "/*KATEX*/", with: script).replacingOccurrences(
      of: "PAYLOAD_BASE64", with: Data(value.utf8).base64EncodedString())
    view.loadHTMLString(html, baseURL: nil)
  }
  static func dismantleUIView(_ view: WKWebView, coordinator: Coordinator) {
    view.configuration.userContentController.removeScriptMessageHandler(forName: "size")
  }
}
