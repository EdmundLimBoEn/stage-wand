import AVFoundation
import MediaPlayer
import UIKit

@MainActor
final class Volume {
    private let onUp: () -> Void
    private let onDown: () -> Void
    private let session = AVAudioSession.sharedInstance()
    private var player: AVAudioPlayer?
    private var observation: NSKeyValueObservation?
    private var notifications: [NSObjectProtocol] = []
    private var volumeWindow: UIWindow?
    private var volumeView: MPVolumeView?
    private var resetTask: Task<Void, Never>?
    private var running = false
    private var previousVolume: Float = 0
    private var lastPress = Date.distantPast
    private var suppressUntil = Date.distantPast
    private(set) var pressCount = 0

    init(onUp: @escaping () -> Void, onDown: @escaping () -> Void) {
        self.onUp = onUp
        self.onDown = onDown
    }

    func start() {
        guard !running else { return }
        running = true
        previousVolume = session.outputVolume
        lastPress = .distantPast
        observation = session.observe(\.outputVolume, options: [.new]) { [weak self] _, change in
            guard let value = change.newValue else { return }
            let observedAt = Date()
            Task { @MainActor [weak self] in
                self?.volumeChanged(to: value, at: observedAt)
            }
        }
        let center = NotificationCenter.default
        notifications.append(center.addObserver(
            forName: AVAudioSession.interruptionNotification, object: session, queue: .main
        ) { [weak self] notification in
            let type = notification.userInfo?[AVAudioSessionInterruptionTypeKey] as? UInt
            guard type == AVAudioSession.InterruptionType.ended.rawValue else { return }
            Task { @MainActor [weak self] in self?.playSilence() }
        })
        notifications.append(center.addObserver(
            forName: AVAudioSession.routeChangeNotification, object: session, queue: .main
        ) { [weak self] _ in
            Task { @MainActor [weak self] in self?.playSilence() }
        })
        playSilence()
        resetSystemVolume()
    }

    func stop() {
        running = false
        observation?.invalidate()
        observation = nil
        notifications.forEach { NotificationCenter.default.removeObserver($0) }
        notifications.removeAll()
        resetTask?.cancel()
        resetTask = nil
        player?.stop()
        player = nil
        volumeWindow?.isHidden = true
        volumeWindow = nil
        volumeView = nil
        do {
            try session.setActive(false, options: .notifyOthersOnDeactivation)
        } catch {
            print("volume audio stop: \(error.localizedDescription)")
        }
    }

    private func playSilence() {
        guard running else { return }
        do {
            try session.setCategory(.playback, options: [.mixWithOthers])
            try session.setActive(true)
            if player == nil {
                guard let url = Bundle.main.url(forResource: "silence", withExtension: "wav") else {
                    print("volume audio: silence.wav is missing")
                    return
                }
                player = try AVAudioPlayer(contentsOf: url)
            }
            guard let player else { return }
            player.numberOfLoops = -1
            player.volume = 0.01
            player.prepareToPlay()
            guard player.play() else {
                print("volume audio: playback failed")
                return
            }
            assert(player.isPlaying)
        } catch {
            print("volume audio: \(error.localizedDescription)")
        }
    }

    private func volumeChanged(to value: Float, at date: Date) {
        guard running, value.isFinite else { return }
        let delta = value - previousVolume
        previousVolume = value
        guard delta != 0, date >= suppressUntil,
              date.timeIntervalSince(lastPress) >= 0.15 else { return }
        lastPress = date
        pressCount += 1
        if delta > 0 {
            print("volume up #\(pressCount)")
            onUp()
        } else {
            print("volume down #\(pressCount)")
            onDown()
        }
        resetSystemVolume()
    }

    private func resetSystemVolume() {
        guard UIApplication.shared.applicationState == .active else { return }
        if volumeWindow == nil {
            guard let scene = UIApplication.shared.connectedScenes
                .compactMap({ $0 as? UIWindowScene })
                .first(where: { $0.activationState == .foregroundActive }) else { return }
            let window = UIWindow(windowScene: scene)
            window.frame = CGRect(x: 0, y: 0, width: 1, height: 1)
            window.isUserInteractionEnabled = false
            let controller = UIViewController()
            controller.view.backgroundColor = .clear
            let view = MPVolumeView(frame: CGRect(x: 0, y: 0, width: 1, height: 1))
            view.alpha = 0.01
            view.showsRouteButton = false
            controller.view.addSubview(view)
            window.rootViewController = controller
            window.isHidden = false
            volumeWindow = window
            volumeView = view
        }
        resetTask?.cancel()
        // MPVolumeView needs a rendered window before its slider can affect volume.
        resetTask = Task { @MainActor [weak self] in
            try? await Task.sleep(for: .milliseconds(100))
            guard !Task.isCancelled, let self, self.running,
                  UIApplication.shared.applicationState == .active,
                  let slider = self.volumeView?.subviews.compactMap({ $0 as? UISlider }).first else { return }
            self.suppressUntil = Date().addingTimeInterval(0.3)
            slider.setValue(0.5, animated: false)
            slider.sendActions(for: .touchUpInside)
            // Read the actual level: iOS may ignore the requested reset entirely.
            self.previousVolume = self.session.outputVolume
        }
    }
}
