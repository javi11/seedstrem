// Renders one frame of the "still downloading" placeholder video.
// Usage: swift placeholder-frame.swift <percent 0-99> <out.png>
// See scripts/gen-placeholders.sh for the full pipeline.
import AppKit

let pct = Int(CommandLine.arguments[1])!
let out = CommandLine.arguments[2]

let w = 1280, h = 720
let img = NSImage(size: NSSize(width: w, height: h))
img.lockFocus()
NSColor(red: 0.055, green: 0.067, blue: 0.094, alpha: 1).setFill()
NSRect(x: 0, y: 0, width: w, height: h).fill()

func draw(_ text: String, size: CGFloat, weight: NSFont.Weight, color: NSColor, y: CGFloat) {
    let attrs: [NSAttributedString.Key: Any] = [
        .font: NSFont.systemFont(ofSize: size, weight: weight),
        .foregroundColor: color,
    ]
    let s = NSAttributedString(string: text, attributes: attrs)
    let sz = s.size()
    s.draw(at: NSPoint(x: (CGFloat(w) - sz.width) / 2, y: y))
}

draw("· · ·", size: 44, weight: .bold, color: NSColor(white: 0.85, alpha: 1), y: 490)
draw("PREPARANDO TU STREAM", size: 56, weight: .bold, color: .white, y: 405)

// Progress bar: track + fill, rounded.
let barW: CGFloat = 720, barH: CGFloat = 16
let barX = (CGFloat(w) - barW) / 2, barY: CGFloat = 350
NSColor(white: 1, alpha: 0.12).setFill()
NSBezierPath(roundedRect: NSRect(x: barX, y: barY, width: barW, height: barH),
             xRadius: barH / 2, yRadius: barH / 2).fill()
if pct > 0 {
    NSColor(red: 0.36, green: 0.65, blue: 1.0, alpha: 1).setFill()
    NSBezierPath(roundedRect: NSRect(x: barX, y: barY, width: barW * CGFloat(pct) / 100, height: barH),
                 xRadius: barH / 2, yRadius: barH / 2).fill()
}

draw("\(pct) % descargado", size: 34, weight: .semibold, color: .white, y: 288)
draw("La descarga está en curso — vuelve en unos minutos", size: 28, weight: .regular,
     color: NSColor(red: 0.73, green: 0.75, blue: 0.80, alpha: 1), y: 230)
draw("Still downloading — come back in a few minutes", size: 22, weight: .regular,
     color: NSColor(red: 0.48, green: 0.51, blue: 0.58, alpha: 1), y: 190)

img.unlockFocus()
guard let tiff = img.tiffRepresentation,
      let rep = NSBitmapImageRep(data: tiff),
      let png = rep.representation(using: .png, properties: [:]) else {
    fatalError("encode failed")
}
try! png.write(to: URL(fileURLWithPath: out))
