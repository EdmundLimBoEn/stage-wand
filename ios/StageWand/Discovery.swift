import Foundation
import Network
import Combine

@MainActor
final class Discovery: NSObject, ObservableObject, @preconcurrency NetServiceDelegate {
    @Published private(set) var results: [(name: String, endpoint: NWEndpoint)] = []
    var onResults: (() -> Void)?
    var onDenied: (() -> Void)?
    private var browser: NWBrowser?
    private var resolver: NWConnection?
    private var resolutionID = UUID()
    private var service: NetService?
    private var completion: (@MainActor (URL?) -> Void)?

    func start() {
        guard browser == nil else { return }
        let browser = NWBrowser(for: .bonjour(type: "_stagewand._tcp", domain: nil), using: .tcp)
        self.browser = browser
        browser.browseResultsChangedHandler = { [weak self] results, _ in
            Task { @MainActor in
                guard let self, self.browser === browser else { return }
                self.results = results.compactMap { result in
                    guard case .service(let name, _, _, _) = result.endpoint else { return nil }
                    return (name: name, endpoint: result.endpoint)
                }.sorted { $0.name < $1.name }
                self.onResults?()
            }
        }
        browser.stateUpdateHandler = { [weak self] state in
            Task { @MainActor in
                guard let self, self.browser === browser else { return }
                switch state {
                case .failed:
                    browser.cancel()
                    self.browser = nil
                    self.onDenied?()
                case .waiting(let error):
                    if case .dns(let code) = error, code == -65570 {
                        browser.cancel()
                        self.browser = nil
                        self.onDenied?()
                    }
                default: break
                }
            }
        }
        browser.start(queue: .main)
    }

    func resolve(_ endpoint: NWEndpoint, completion: @escaping @MainActor (URL?) -> Void) {
        cancelResolution()
        if case .service(let name, let type, let domain, _) = endpoint {
            let service = NetService(domain: domain, type: type, name: name)
            self.service = service
            self.completion = completion
            service.delegate = self
            service.schedule(in: .main, forMode: .common)
            service.resolve(withTimeout: 2)
            return
        }
        let id = resolutionID
        let connection = NWConnection(to: endpoint, using: .tcp)
        resolver = connection
        connection.stateUpdateHandler = { [weak self] state in
            Task { @MainActor in
                guard let self, self.resolutionID == id else { return }
                switch state {
                case .ready:
                    let url: URL?
                    if case .hostPort(let host, let port) = connection.currentPath?.remoteEndpoint {
                        var parts = URLComponents()
                        parts.scheme = "ws"
                        let address = "\(host)"
                        parts.host = address.contains(":") && !address.hasPrefix("[") ? "[\(address)]" : address
                        parts.port = Int(port.rawValue)
                        parts.path = "/"
                        url = parts.url
                    } else {
                        url = nil
                    }
                    self.cancelResolution()
                    completion(url)
                case .failed:
                    self.cancelResolution()
                    completion(nil)
                default: break
                }
            }
        }
        connection.start(queue: .main)
    }

    func netServiceDidResolveAddress(_ sender: NetService) {
        guard sender === service else { return }
        var parts = URLComponents()
        parts.scheme = "ws"
        parts.host = sender.hostName
        parts.port = sender.port
        parts.path = "/"
        let callback = completion
        let url = parts.url
        cancelResolution()
        callback?(url)
    }

    func netService(_ sender: NetService, didNotResolve errorDict: [String: NSNumber]) {
        guard sender === service else { return }
        let callback = completion
        cancelResolution()
        callback?(nil)
    }

    func cancelResolution() {
        service?.stop()
        service?.delegate = nil
        service = nil
        completion = nil
        resolutionID = UUID()
        resolver?.cancel()
        resolver = nil
    }
}
