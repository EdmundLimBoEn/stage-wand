import Foundation
import Network
import Combine

@MainActor
final class Discovery: ObservableObject {
    @Published private(set) var results: [(name: String, endpoint: NWEndpoint)] = []
    var onResults: (() -> Void)?
    var onDenied: (() -> Void)?
    private var browser: NWBrowser?
    private var resolver: NWConnection?
    private var resolutionID = UUID()

    func start() {
        guard browser == nil else { return }
        let parameters = NWParameters.tcp
        parameters.includePeerToPeer = true
        let browser = NWBrowser(for: .bonjour(type: "_stagewand._tcp", domain: nil), using: parameters)
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


    // Native WebSocket needs a URL endpoint; resolve over peer-to-peer TCP first and retain its interface.
    func resolve(_ endpoint: NWEndpoint, completion: @escaping @MainActor (URL?, NWInterface?) -> Void) {
        cancelResolution()
        let id = resolutionID
        let parameters = NWParameters.tcp
        parameters.includePeerToPeer = true
        let connection = NWConnection(to: endpoint, using: parameters)
        resolver = connection
        connection.stateUpdateHandler = { [weak self] state in
            Task { @MainActor in
                guard let self, self.resolutionID == id else { return }
                switch state {
                case .ready:
                    var url: URL?
                    if case .hostPort(let host, let port) = connection.currentPath?.remoteEndpoint {
                        var parts = URLComponents()
                        parts.scheme = "ws"
                        let address = "\(host)"
                        parts.host = address.contains(":") && !address.hasPrefix("[") ? "[\(address)]" : address
                        parts.port = Int(port.rawValue)
                        parts.path = "/"
                        url = parts.url
                    }
                    let interface = connection.currentPath?.availableInterfaces.first
                    self.cancelResolution()
                    completion(url, interface)
                case .failed:
                    self.cancelResolution()
                    completion(nil, nil)
                default: break
                }
            }
        }
        connection.start(queue: .main)
    }

    func cancelResolution() {
        resolutionID = UUID()
        resolver?.cancel()
        resolver = nil
    }
}
