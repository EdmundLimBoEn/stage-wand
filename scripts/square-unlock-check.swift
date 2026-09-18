import Foundation

@main
struct SquareUnlockCheck {
    static func main() {
        func trace(duration: Double = 2) -> SquareUnlock {
            var recognizer = SquareUnlock()
            for step in 0...40 {
                let p = Double(step) / 10
                let point: (Double, Double)
                if p <= 1 { point = (p, 0) }
                else if p <= 2 { point = (1, p - 1) }
                else if p <= 3 { point = (3 - p, 1) }
                else { point = (0, 4 - p) }
                recognizer.add(x: point.0, y: point.1, at: duration * p / 4)
            }
            return recognizer
        }
        var valid = trace()
        precondition(valid.finish(x: 0, y: 0, at: 2), "Valid square rejected")
        var fast = trace(duration: 0.2)
        precondition(!fast.finish(x: 0, y: 0, at: 0.2), "Fast gesture accepted")
        var tap = SquareUnlock()
        tap.add(x: 0, y: 0, at: 0)
        precondition(!tap.finish(x: 0, y: 0, at: 2), "Tap accepted")
        var partial = SquareUnlock()
        for i in 0...10 { partial.add(x: Double(i) / 10, y: 0, at: Double(i) / 10) }
        precondition(!partial.finish(x: 1, y: 0, at: 2), "Partial accepted")
        var diagonal = SquareUnlock()
        diagonal.add(x: 0, y: 0, at: 0)
        diagonal.add(x: 0.5, y: 0.5, at: 1)
        precondition(!diagonal.finish(x: 0, y: 0, at: 2), "Diagonal accepted")
        var wrongStart = SquareUnlock()
        wrongStart.add(x: 1, y: 0, at: 0)
        precondition(wrongStart.failed, "Wrong start accepted")
        diagonal.reset()
        precondition(!diagonal.failed && diagonal.progress == 0, "Reset retained trace")
        precondition(!diagonal.finish(x: 0, y: 0, at: 3), "Reset accepted stale gesture")
        var scribble = SquareUnlock()
        scribble.add(x: 0, y: 0, at: 0)
        for i in 1...20 { scribble.add(x: i.isMultiple(of: 2) ? 0 : 0.1, y: 0, at: Double(i) / 10) }
        precondition(scribble.failed, "Scribble accepted")
        print("Square unlock checks passed: valid, fast, tap, partial, diagonal, wrong start, reset, scribble")
    }
}
